package resolver

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"strconv"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Fallback mode: when the DoH server isn't working, or the machine has no IPv4 route,
// every name goes to the OS until a probe to the DoH server is usable again. One
// background loop re-checks the route and sends the probes.

const (
	// failureLimit is how many DoH queries in a row have to fail before the loop starts.
	failureLimit = 5
	// probeTimeout limits a probe sent while names still go to the DoH server, where each
	// lookup waits out DoH's share of the time until the probe ends. It matches that share
	// with Go's default timeout.
	probeTimeout = 3 * time.Second
	// recoveryProbeTimeout limits a probe sent while every name goes to the OS, when
	// nothing waits on it. It matches the DoH client's connect timeout. On a link where
	// connecting takes longer than probeTimeout, each failed probe's reset would otherwise
	// throw away the connection being set up, and DoH would never come back.
	recoveryProbeTimeout = 10 * time.Second
	// firstWait is the wait after the loop's first round. Each later wait is twice as
	// long, up to maxWait.
	firstWait = time.Second
	maxWait   = time.Minute
)

// probeQuestion is what a probe asks: the address of the first root server. It is an
// ordinary address lookup, and its answer depends only on the root servers' own zone,
// which is unsigned. A name such as example.com would depend on its own DNS hosting and
// DNSSEC signing: if those broke, every provider would answer the probe with SERVFAIL,
// and every user would fall back to plaintext DNS at once. The root name itself, such
// as ". NS", isn't used because some servers reject any question about it: CleanBrowsing
// answers with a message that isn't a response.
//
// Two limits remain. The record is cached for days, so a server that is reachable but
// can't resolve other names, such as a forwarder whose upstream is down, still passes.
// Each name then waits out DoH's share of the time before it goes to the OS. And any
// fixed question can hit a quirk of an untested custom server, which then stays in
// fallback mode until Zen restarts.
var probeQuestion = dnsmessage.Question{
	Name:  dnsmessage.MustNewName("a.root-servers.net."),
	Type:  dnsmessage.TypeA,
	Class: dnsmessage.ClassINET,
}

// state says whether names can go to the DoH server.
type state int

const (
	// stateWorking means names go to the DoH server, by the routing rules.
	stateWorking state = iota
	// stateNoIPv4 means the machine has no IPv4 route, and every name goes to the OS.
	stateNoIPv4
	// stateFailing means the DoH server isn't working, and every name goes to the OS.
	stateFailing
)

// StatusMode says how the resolver answers names.
type StatusMode string

const (
	StatusActive  StatusMode = "active"  // names go to the DoH server
	StatusVPN     StatusMode = "vpn"     // a VPN handles all DNS
	StatusNoIPv4  StatusMode = "noIPv4"  // the machine has no IPv4 route
	StatusFailing StatusMode = "failing" // the DoH server isn't working
)

// Status is what the resolver reports to the onStatus callback given to [New].
type Status struct {
	Mode StatusMode
	// Error says what went wrong with the last probe, in English. It is set only with
	// StatusFailing. It can hold a custom server's URL or host. A certificate error lists
	// the domains of the certificate the server presented, which can identify the network
	// the user is on, such as a login page's.
	Error string
}

// countFailure counts a DoH query that says the server isn't working, and starts the
// loop when enough have come in a row. A probe then decides whether the server is
// working, because whoever runs a domain's DNS can make lookups of it fail, and a web
// page could otherwise push Zen onto the OS resolver.
func (r *Resolver) countFailure() {
	if r.failures.Add(1) < failureLimit {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startLoopLocked()
}

// startLoopLocked starts the loop, unless it is running or the resolver is closed.
// r.mu must be held.
func (r *Resolver) startLoopLocked() {
	if r.loopRunning || r.closed {
		return
	}
	r.loopRunning = true
	go r.loop()
}

// kickLocked asks the loop to run its next round at once. A kick while no loop runs is
// harmless: the next loop's first round takes it. r.mu must be held.
func (r *Resolver) kickLocked() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// loop runs rounds until a probe is usable or the resolver is closed. The first round
// runs at once, and the waits between rounds double from firstWait up to maxWait. The
// waits use ordinary timers, which stop while the machine sleeps. A wake reported as a
// network change runs a round at once.
func (r *Resolver) loop() {
	wait := firstWait
	for {
		done, kicked := r.round()
		if done {
			return
		}
		if kicked {
			// The kick came after the last wait ended but before the round began, so the
			// select below won't see it.
			wait = firstWait
		}
		select {
		case <-time.After(wait):
			wait = min(2*wait, maxWait)
		case <-r.kick:
			wait = firstWait
		case <-r.loopCtx.Done():
			return
		}
	}
}

// round checks the route and, with a route and no VPN handling all DNS, probes the DoH
// server. Then it sets the state from what it found, unless Close or NetworkChanged
// ended the round meanwhile. Such a round changes nothing, not even the client.
//
// done reports whether the loop should end: the probe was usable, or the resolver is
// closed. kicked reports whether the round took a kick that came before it.
func (r *Resolver) round() (done, kicked bool) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return true, false
	}
	// A kick that came before this round has been answered by it. Taking it here, before
	// the VPN flag is read, means a later change of the flag always leaves a kick.
	select {
	case <-r.kick:
		kicked = true
	default:
	}
	ctx, cancel := context.WithCancel(r.loopCtx)
	r.cancelRound = cancel
	timeout := probeTimeout
	if r.state != stateWorking {
		timeout = recoveryProbeTimeout
	}
	r.mu.Unlock()
	defer cancel()

	hasRoute := r.hasRoute()
	vpn := r.routing.Load().vpnHandlesAllDNS
	var probeErr error
	if hasRoute && !vpn {
		probeErr = r.probe(ctx, timeout)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil {
		// Close or NetworkChanged ended the round. NetworkChanged also kicked the loop, so
		// the next round runs at once and the waits start again from firstWait. Anything
		// else that cancels a round has to kick too.
		return false, kicked
	}

	switch {
	case !hasRoute:
		r.setStateLocked(stateNoIPv4, "")
	case vpn:
		// A VPN handles all DNS, so DoH isn't used, and a probe would only send DoH
		// traffic through the VPN. SetRouting kicks the loop when the flag clears.
	case probeErr == nil:
		// The count is reset before DoH comes back on, so that a failure of the first
		// query after that counts.
		r.failures.Store(0)
		r.setStateLocked(stateWorking, "")
		r.loopRunning = false
		return true, kicked
	default:
		if _, answered := errors.AsType[rcodeError](probeErr); !answered {
			// The probe uses the same connection as the lookups, and that connection can
			// be the problem: it can be dead without the OS noticing, while a new one
			// would work. Resetting only after a failed probe, rather than before each,
			// keeps web pages from forcing reconnects with a few slow names.
			r.client.Reset()
		}
		r.setStateLocked(stateFailing, statusError(probeErr))
	}
	return false, kicked
}

// probe sends one DoH query for a name no web page controls, past routing and the
// cache, within timeout. It returns nil if the server answered usably. NXDOMAIN is
// usable: the server answered.
func (r *Resolver) probe(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	msg, err := r.client.Query(ctx, probeQuestion)
	if err != nil {
		return err
	}
	if interpret(msg, probeQuestion).failed {
		return rcodeError(msg.RCode)
	}
	return nil
}

// rcodeError is the error for a probe's response that counts against the server. It
// holds the response's rcode.
type rcodeError dnsmessage.RCode

func (e rcodeError) Error() string {
	// dnsmessage.RCode.String returns names such as RCodeServerFailure.
	switch dnsmessage.RCode(e) {
	case dnsmessage.RCodeFormatError:
		return "server answered FORMERR"
	case dnsmessage.RCodeServerFailure:
		return "server answered SERVFAIL"
	case dnsmessage.RCodeNotImplemented:
		return "server answered NOTIMP"
	case dnsmessage.RCodeRefused:
		return "server answered REFUSED"
	}
	return "server answered rcode " + strconv.Itoa(int(e))
}

// statusError turns a failed probe's error into the status's Error. The text has to stay
// the same from one probe to the next, or each probe would report a new status.
func statusError(err error) string {
	// On a network that drops packets, the first probe ends with its context's deadline,
	// and later ones with the client's recorded dial error, "dial tcp …: i/o timeout".
	// errors.Is(err, context.DeadlineExceeded) doesn't reliably match the second.
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return "timed out"
	}
	// On Linux, Go checks certificates itself, and the text of this error holds the
	// current time. macOS and Windows check them with the OS, whose text is stable.
	if certErr, ok := errors.AsType[x509.CertificateInvalidError](err); ok && certErr.Reason == x509.Expired {
		return "x509: certificate has expired or is not yet valid"
	}
	// A *net.OpError prints both ends of the connection, and the local port differs on
	// each attempt. Printing it without them also keeps the machine's own address out.
	if opErr, ok := errors.AsType[*net.OpError](err); ok && opErr.Err != nil {
		return (&net.OpError{Op: opErr.Op, Net: opErr.Net, Err: opErr.Err}).Error()
	}
	return err.Error()
}

// setStateLocked sets the state and reports the status. r.mu must be held.
func (r *Resolver) setStateLocked(s state, errText string) {
	r.state, r.stateErr = s, errText
	r.dohOff.Store(s != stateWorking)
	r.reportLocked()
}

// reportLocked gives the status to onStatus if it differs from the last one, unless the
// resolver is closed. Calling onStatus with r.mu held keeps the calls in order, so the
// last one always carries the current status. r.mu must be held.
func (r *Resolver) reportLocked() {
	if r.closed {
		return
	}
	var s Status
	switch {
	case r.routing.Load().vpnHandlesAllDNS:
		s.Mode = StatusVPN
	case r.state == stateNoIPv4:
		s.Mode = StatusNoIPv4
	case r.state == stateFailing:
		s = Status{Mode: StatusFailing, Error: r.stateErr}
	default:
		s.Mode = StatusActive
	}
	if s == r.reported {
		return
	}
	r.reported = s
	if r.onStatus != nil {
		r.onStatus(s)
	}
}

// hasIPv4Route reports whether the machine has an IPv4 route to the internet. A UDP dial
// looks up the route without sending anything. Go's resolver makes the same kind of dial
// to sort addresses (net/addrselect.go). Any error counts as no route, including one
// with another cause, such as running out of file descriptors: the next round corrects
// it, and both states send names to the OS meanwhile.
func hasIPv4Route() bool {
	conn, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

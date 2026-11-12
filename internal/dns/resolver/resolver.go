// Package resolver answers the proxy's DNS lookups through a DoH server, or through the
// OS resolver for names that DoH can't or shouldn't answer.
package resolver

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	"github.com/irbis-sh/zen-desktop/internal/dns/doh"
	"golang.org/x/net/dns/dnsmessage"
)

// dohClient is the part of [doh.Client] the resolver uses.
type dohClient interface {
	Query(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error)
	Reset()
	Close()
}

// Resolver decides where each name goes and answers Go's DNS queries. Create one with
// [New]. A Resolver is safe for concurrent use by multiple goroutines.
type Resolver struct {
	client   dohClient
	lookupOS func(ctx context.Context, host string) ([]netip.Addr, error)
	// hasRoute reports whether the machine has an IPv4 route (see hasIPv4Route).
	hasRoute func() bool
	onStatus func(Status)
	cache    *cache
	// osDomains holds the domains that always go to the OS. It doesn't change after New.
	osDomains map[string]struct{}
	// routing is replaced as a whole, because the network monitor calls SetRouting from
	// its own goroutine while lookups read the state. It is stored under mu, which keeps
	// the reported status in step with it.
	routing atomic.Pointer[routing]
	// netResolver is shared, because Go merges concurrent lookups of a name only within
	// one net.Resolver.
	netResolver *net.Resolver

	// failures counts the DoH queries in a row that say the server isn't working. Lookups
	// update it without taking mu.
	failures atomic.Int32
	// dohOff is true when state isn't stateWorking, so that lookups can read it without
	// taking mu.
	dohOff atomic.Bool
	// kick asks the loop to run its next round at once and to go back to the shortest
	// wait. It holds at most one request.
	kick chan struct{}
	// loopCtx ends when the resolver is closed. Each round's context derives from it.
	loopCtx    context.Context
	cancelLoop context.CancelFunc

	mu    sync.Mutex
	state state
	// stateErr says why the last probe failed. It is set only with stateFailing.
	stateErr    string
	loopRunning bool
	// cancelRound ends the latest round's context. It is nil until the first round.
	cancelRound context.CancelFunc
	// reported is the last status given to onStatus. It starts with an empty Mode, so
	// the first status is always reported.
	reported Status
	closed   bool
}

// New returns a resolver that sends names to client. systemDomains holds the user's
// "Use system DNS" domains, normalised by the config: lower-cased, without a leading
// "*." or "." and without a trailing dot.
//
// onStatus, if not nil, gets the first status before New returns, and after that each
// status that differs from the last one. It is called one call at a time, on the
// goroutine that caused the change, with the resolver's lock held. So it must return
// quickly, must not call the resolver, and must not take a lock that a caller of the
// resolver holds while calling it. It is never called after [Resolver.Close] returns.
func New(client *doh.Client, systemDomains []string, onStatus func(Status)) *Resolver {
	return newResolver(client, systemDomains, lookupOS, hasIPv4Route, onStatus)
}

func lookupOS(ctx context.Context, host string) ([]netip.Addr, error) {
	// Go asks for A and AAAA at the same time. With the same network for both,
	// DefaultResolver merges them into one OS lookup.
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// newResolver returns a resolver like [New] does, from any DoH client, OS lookup and
// route check, so that tests can replace them.
func newResolver(
	client dohClient,
	systemDomains []string,
	lookupOS func(ctx context.Context, host string) ([]netip.Addr, error),
	hasRoute func() bool,
	onStatus func(Status),
) *Resolver {
	r := &Resolver{
		client:    client,
		lookupOS:  lookupOS,
		hasRoute:  hasRoute,
		onStatus:  onStatus,
		cache:     newCache(),
		osDomains: make(map[string]struct{}, len(reservedDomains)+len(systemDomains)),
		kick:      make(chan struct{}, 1),
	}
	for _, d := range reservedDomains {
		r.osDomains[d] = struct{}{}
	}
	for _, d := range systemDomains {
		r.osDomains[d] = struct{}{}
	}
	r.routing.Store(&routing{})
	r.netResolver = &net.Resolver{PreferGo: true, Dial: r.dial}
	r.loopCtx, r.cancelLoop = context.WithCancel(context.Background())

	r.mu.Lock()
	defer r.mu.Unlock()
	s := stateWorking
	if !r.hasRoute() {
		s = stateNoIPv4
		r.startLoopLocked()
	}
	r.setStateLocked(s, "")
	return r
}

// NetResolver returns the [net.Resolver] the proxy dials with.
func (r *Resolver) NetResolver() *net.Resolver {
	return r.netResolver
}

// NetworkChanged resets the DoH client, so that later queries use new connections, and
// clears the cache. It then checks the IPv4 route again. If the background loop is
// running, NetworkChanged abandons its round and has the next one run at once.
func (r *Resolver) NetworkChanged() {
	// The reset comes before the next round's probe, so that the probe uses a new
	// connection.
	r.client.Reset()
	r.cache.clear()
	hasRoute := r.hasRoute()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		// No loop could turn DoH back on, and lookups still work after Close.
		return
	}
	if !hasRoute {
		r.setStateLocked(stateNoIPv4, "")
		r.startLoopLocked()
	}
	// A round from the old network could otherwise write over the new state: a probe
	// that times out after the route went away would report failing, and one that comes
	// back usable would turn DoH on without a route.
	if r.cancelRound != nil {
		r.cancelRound()
	}
	r.kickLocked()
}

// Close stops the background loop and closes the DoH client's idle connections. After
// Close, the resolver reports no status, but lookups still work.
func (r *Resolver) Close() {
	// Under r.mu, so that a round's context ends in the same step as closed is set, and
	// round's one check of its context covers Close as well as NetworkChanged.
	r.mu.Lock()
	r.closed = true
	r.cancelLoop()
	r.mu.Unlock()
	r.client.Close()
}

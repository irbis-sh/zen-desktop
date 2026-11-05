package resolver

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

const (
	// defaultBudget is the time for one exchange if the context has no deadline. It matches
	// Go's default.
	defaultBudget = 5 * time.Second
	// answerMargin is how long before Go's deadline the resolver gives up and answers
	// SERVFAIL, so that Go gets an answer rather than a timeout.
	answerMargin = 100 * time.Millisecond
)

// dial is the [net.Resolver] Dial hook. It returns an in-memory connection and serves
// Go's query on the other end. Go treats that connection like a DNS server from
// resolv.conf. A pipe isn't a net.PacketConn, so Go uses TCP framing even when network is
// "udp", and it dials again for each retry.
func (r *Resolver) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	// Go keeps dialling for its remaining retries after a lookup is cancelled. serve's
	// AfterFunc would close such a pipe on its own goroutine, and serve can read the query
	// and start a lookup before it does.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, server := net.Pipe()
	go r.serve(ctx, server) // #nosec G118 -- serve drops ctx's values on purpose; see its comment
	return client, nil
}

// serve answers the one query Go sends on conn, and closes conn. Go doesn't watch ctx
// during an exchange: it reads from conn until its deadline, so conn has to close as soon
// as ctx ends.
func (r *Resolver) serve(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	start := time.Now()
	budget := defaultBudget
	if deadline, ok := ctx.Deadline(); ok {
		budget = deadline.Sub(start)
	}
	// DoH gets 60% of the budget. The rest is left for the OS, if DoH doesn't answer.
	dohDeadline := start.Add(budget * 6 / 10)

	// The lookups don't run on ctx, because ctx carries the values of the request that
	// triggered this lookup. Those include the proxy's httptrace hooks, which a DoH request
	// would then call.
	lookupCtx, cancel := context.WithDeadline(context.Background(), start.Add(budget-answerMargin))
	defer cancel()
	stop := context.AfterFunc(ctx, func() {
		// In this order, because cancel lets a lookup fail, and serve would then write a
		// SERVFAIL if conn were still open.
		conn.Close()
		cancel()
	})
	defer stop()

	raw, err := readMessage(conn)
	if err != nil {
		return
	}
	var query dnsmessage.Message
	if err := query.Unpack(raw); err != nil || len(query.Questions) != 1 {
		return
	}

	rcode, addrs := r.resolve(lookupCtx, query.Questions[0], dohDeadline)
	answer, err := response(&query, rcode, addrs).Pack()
	if err != nil {
		return
	}
	writeMessage(conn, answer)
}

// resolve answers q. ctx ends when the answer has to be ready, and dohDeadline when DoH
// has to give way to the OS.
func (r *Resolver) resolve(ctx context.Context, q dnsmessage.Question, dohDeadline time.Time) (dnsmessage.RCode, []netip.Addr) {
	if q.Class != dnsmessage.ClassINET || (q.Type != dnsmessage.TypeA && q.Type != dnsmessage.TypeAAAA) {
		return dnsmessage.RCodeNotImplemented, nil
	}

	name := strings.ToLower(strings.TrimSuffix(q.Name.String(), "."))
	if r.useOS(name) {
		return r.askOS(ctx, name, q.Type)
	}

	key := cacheKey{name: name, qtype: q.Type}
	v, ok := r.cache.get(key)
	if !ok {
		dohCtx, cancel := context.WithDeadline(ctx, dohDeadline)
		msg, err := r.client.Query(dohCtx, q)
		cancel()
		if err != nil {
			return r.askOS(ctx, name, q.Type)
		}
		v = interpret(msg, q)
		// This comes before the answer is written, because Go retries a SERVFAIL at once
		// and the retry has to find it.
		r.cache.put(key, v)
	}
	if v.useOS {
		return r.askOS(ctx, name, q.Type)
	}
	return v.rcode, v.addrs
}

// askOS answers a question of type qtype about name from the OS resolver. The OS caches
// answers itself, so they aren't cached here.
func (r *Resolver) askOS(ctx context.Context, name string, qtype dnsmessage.Type) (dnsmessage.RCode, []netip.Addr) {
	// With ctx already over, the lookup could still go out unencrypted, for nothing.
	if ctx.Err() != nil {
		return dnsmessage.RCodeServerFailure, nil
	}

	addrs, err := r.lookupOS(ctx, name)
	if err != nil {
		if dnsErr, ok := errors.AsType[*net.DNSError](err); ok && dnsErr.IsNotFound {
			return dnsmessage.RCodeNameError, nil
		}
		return dnsmessage.RCodeServerFailure, nil
	}

	var matching []netip.Addr
	for _, a := range addrs {
		if a = a.Unmap(); a.Is4() == (qtype == dnsmessage.TypeA) {
			matching = append(matching, a)
		}
	}
	return dnsmessage.RCodeSuccess, matching
}

// response builds the answer to query, whose question is its only one.
//
// Go's address lookup reads only address records and never TTLs, so there is nothing else
// to send. With RA set, Go accepts an NXDOMAIN or an empty NOERROR without an SOA, and
// doesn't retry them as a lame referral.
func response(query *dnsmessage.Message, rcode dnsmessage.RCode, addrs []netip.Addr) *dnsmessage.Message {
	q := query.Questions[0]
	msg := &dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 query.ID,
			Response:           true,
			RecursionDesired:   query.RecursionDesired,
			RecursionAvailable: true,
			RCode:              rcode,
		},
		Questions: query.Questions,
	}
	for _, a := range addrs {
		rr := dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: q.Type, Class: q.Class}}
		if q.Type == dnsmessage.TypeA {
			rr.Body = &dnsmessage.AResource{A: a.As4()}
		} else {
			rr.Body = &dnsmessage.AAAAResource{AAAA: a.As16()}
		}
		msg.Answers = append(msg.Answers, rr)
	}
	return msg
}

// readMessage reads a DNS message with TCP framing: a 2-byte length, then the message.
func readMessage(r io.Reader) ([]byte, error) {
	var length [2]byte
	if _, err := io.ReadFull(r, length[:]); err != nil {
		return nil, err
	}
	msg := make([]byte, binary.BigEndian.Uint16(length[:]))
	if _, err := io.ReadFull(r, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// writeMessage writes a DNS message with TCP framing.
func writeMessage(w io.Writer, msg []byte) error {
	if len(msg) > 65535 {
		return errors.New("message too long")
	}
	framed := binary.BigEndian.AppendUint16(nil, uint16(len(msg))) // #nosec G115 -- checked above
	_, err := w.Write(append(framed, msg...))
	return err
}

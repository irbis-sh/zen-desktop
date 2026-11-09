// Package resolver answers the proxy's DNS lookups through a DoH server, or through the
// OS resolver for names that DoH can't or shouldn't answer.
package resolver

import (
	"context"
	"net"
	"net/netip"
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
	cache    *cache
	// osDomains holds the domains that always go to the OS. It doesn't change after New.
	osDomains map[string]struct{}
	// routing is replaced as a whole, because the network monitor calls SetRouting from
	// its own goroutine while lookups read the state.
	routing atomic.Pointer[routing]
	// netResolver is shared, because Go merges concurrent lookups of a name only within
	// one net.Resolver.
	netResolver *net.Resolver
}

// New returns a resolver that sends names to client. systemDomains holds the user's
// "Use system DNS" domains, normalised by the config: lower-cased, without a leading
// "*." or "." and without a trailing dot.
func New(client *doh.Client, systemDomains []string) *Resolver {
	return newResolver(client, systemDomains, func(ctx context.Context, host string) ([]netip.Addr, error) {
		// Go asks for A and AAAA at the same time. With the same network for both,
		// DefaultResolver merges them into one OS lookup.
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	})
}

// newResolver returns a resolver like [New] does, from any DoH client and OS lookup, so
// that tests can replace them.
func newResolver(
	client dohClient,
	systemDomains []string,
	lookupOS func(ctx context.Context, host string) ([]netip.Addr, error),
) *Resolver {
	r := &Resolver{
		client:    client,
		lookupOS:  lookupOS,
		cache:     newCache(),
		osDomains: make(map[string]struct{}, len(reservedDomains)+len(systemDomains)),
	}
	for _, d := range reservedDomains {
		r.osDomains[d] = struct{}{}
	}
	for _, d := range systemDomains {
		r.osDomains[d] = struct{}{}
	}
	r.routing.Store(&routing{})
	r.netResolver = &net.Resolver{PreferGo: true, Dial: r.dial}
	return r
}

// NetResolver returns the [net.Resolver] the proxy dials with.
func (r *Resolver) NetResolver() *net.Resolver {
	return r.netResolver
}

// NetworkChanged resets the DoH client, so that later queries use new connections, and
// clears the cache.
func (r *Resolver) NetworkChanged() {
	r.client.Reset()
	r.cache.clear()
}

// Close closes the DoH client's idle connections.
func (r *Resolver) Close() {
	r.client.Close()
}

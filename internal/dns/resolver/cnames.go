package resolver

import (
	"context"
	"net/netip"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// CNAMEs returns host's CNAME chain from the last DoH answer for it: the target of each
// CNAME record followed, nearest first. That answer may have expired up to maxChainAge
// ago, because connections dialled from it can still be open. Without such an answer,
// CNAMEs looks host up first, which can take as long as ctx allows.
//
// It returns nil for a host that the OS answers, including every host in fallback mode
// or while a VPN handles all DNS. The caller must not change the returned slice.
func (r *Resolver) CNAMEs(ctx context.Context, host string) []string {
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	// An IP address with dots isn't routed to the OS by name, and would otherwise be
	// sent to the DoH server as one.
	if _, err := netip.ParseAddr(name); err == nil || r.useOS(name) {
		return nil
	}
	if chain, ok := r.cachedChain(name); ok {
		return chain
	}

	// Through Go's resolver rather than straight to the DoH client: Go reads the hosts
	// file first, so a name from it gets no chain, as it is dialled at the file's
	// address, and Go merges concurrent lookups of a name. The lookup fills the cache,
	// so its result isn't needed. The rooted name stops Go from trying search domains.
	_, _ = r.netResolver.LookupNetIP(ctx, "ip", name+".")
	chain, _ := r.cachedChain(name)
	return chain
}

// cachedChain returns the chain of the cached A answer for name, or of the AAAA answer
// when there is no A answer. ok is false when neither is cached. The chain is nil when
// the cached verdict sends name to the OS.
func (r *Resolver) cachedChain(name string) (chain []string, ok bool) {
	for _, qtype := range [...]dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
		if v, found := r.cache.getStale(cacheKey{name: name, qtype: qtype}); found {
			return v.chain, true
		}
	}
	return nil, false
}

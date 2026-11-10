package resolver

import (
	"strings"

	"golang.org/x/net/publicsuffix"
)

// reservedDomains are the domains that only make sense on the local network.
var reservedDomains = []string{"localhost", "local", "home.arpa", "internal", "lan"}

// routing is what the network announces.
type routing struct {
	domains          map[string]struct{}
	vpnHandlesAllDNS bool
}

// SetRouting sets the search and match domains the network announces, lower-cased and
// without trailing dots, and whether a VPN answers every name.
//
// Domains that are public suffixes are dropped: a network that announces com would
// otherwise send most lookups to the OS. The user's own domains aren't filtered, so a
// network that uses dev internally can still be covered there.
func (r *Resolver) SetRouting(domains []string, vpnHandlesAllDNS bool) {
	kept := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		// The root arrives as "" once its dot is trimmed. under wouldn't match it today, but
		// a match on the root would send every name to the OS.
		if d != "" && !isPublicSuffix(d) {
			kept[d] = struct{}{}
		}
	}
	r.routing.Store(&routing{domains: kept, vpnHandlesAllDNS: vpnHandlesAllDNS})
}

// isPublicSuffix reports whether d is a public suffix. That takes more than
// publicsuffix.PublicSuffix(d) == d, because for an unlisted name such as corp,
// PublicSuffix applies the default rule and returns ("corp", false).
func isPublicSuffix(d string) bool {
	suffix, icann := publicsuffix.PublicSuffix(d)
	return suffix == d && (icann || strings.Contains(d, "."))
}

// useOS reports whether name, lower-cased and without the trailing dot, skips DoH.
func (r *Resolver) useOS(name string) bool {
	state := r.routing.Load()
	return state.vpnHandlesAllDNS ||
		!strings.Contains(name, ".") ||
		under(name, r.osDomains) ||
		under(name, state.domains)
}

// under reports whether name is one of domains or a subdomain of one.
func under(name string, domains map[string]struct{}) bool {
	for {
		if _, ok := domains[name]; ok {
			return true
		}
		_, parent, ok := strings.Cut(name, ".")
		if !ok {
			return false
		}
		name = parent
	}
}

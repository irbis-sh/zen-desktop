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
// without trailing dots, and whether a VPN answers every name. A change of the VPN flag
// changes the status too.
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

	r.mu.Lock()
	defer r.mu.Unlock()
	wasVPN := r.routing.Load().vpnHandlesAllDNS
	r.routing.Store(&routing{domains: kept, vpnHandlesAllDNS: vpnHandlesAllDNS})
	r.reportLocked()
	if wasVPN && !vpnHandlesAllDNS {
		// The loop sends no probes while a VPN handles all DNS. Without a round now, the
		// status could show the state from before the VPN for up to maxWait.
		r.kickLocked()
	}
}

// isPublicSuffix reports whether d is a public suffix. That takes more than
// publicsuffix.PublicSuffix(d) == d, because for an unlisted name such as corp,
// PublicSuffix applies the default rule and returns ("corp", false).
func isPublicSuffix(d string) bool {
	suffix, icann := publicsuffix.PublicSuffix(d)
	return suffix == d && (icann || strings.Contains(d, "."))
}

// useOS reports whether name, lower-cased and without the trailing dot, skips DoH. In
// fallback mode, or without an IPv4 route, every name does.
func (r *Resolver) useOS(name string) bool {
	rt := r.routing.Load()
	return r.dohOff.Load() ||
		rt.vpnHandlesAllDNS ||
		!strings.Contains(name, ".") ||
		under(name, r.osDomains) ||
		under(name, rt.domains)
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

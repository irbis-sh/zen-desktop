package resolver

import (
	"encoding/binary"
	"math"
	"net/netip"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

const (
	// maxChain is how many CNAME records the resolver follows. Real chains are far shorter,
	// so this only bounds what a hostile zone can cost, and it also ends loops.
	maxChain = 16
	// maxAddrs is how many addresses an answer keeps. One DoH response can hold about 4,000,
	// which would let a hostile zone fill the cache with about 1 GB of addresses. Only the
	// kept ones are checked for blocks and local addresses, so one listed after them goes
	// unnoticed. That also takes a hostile zone, and the address never reaches Go.
	maxAddrs = 32
	// minTTL keeps every answer for a while. Without it, a TTL of 0 would make each new
	// connection wait for a DoH round trip. It also means Go's retries of a SERVFAIL reach
	// the server once: Go resends one up to attempts × servers times, for A and AAAA alike.
	minTTL = 10 * time.Second
	// maxNegativeTTL caps how long a name stays reported as missing or blocked.
	maxNegativeTTL = 5 * time.Minute

	// optionEDE is the EDNS(0) option code of an Extended DNS Error (RFC 8914).
	optionEDE = 15
)

// verdict says how to answer a question, from a DoH response.
type verdict struct {
	rcode dnsmessage.RCode
	addrs []netip.Addr
	// useOS means the OS resolver answers instead, and rcode and addrs mean nothing.
	useOS bool
	// ttl is how long the verdict can be cached.
	ttl time.Duration
}

// interpret turns the response to q into a verdict. The order of the cases matters: a block,
// whether an EDE or an unspecified address marks it, outranks any rcode, and other addresses
// count only in a successful response.
func interpret(msg *dnsmessage.Message, q dnsmessage.Question) verdict {
	addrs, addrTTL := findAddrs(msg, q)
	blocked, dnssecFailed := extendedErrors(msg)

	switch {
	case blocked || slices.ContainsFunc(addrs, isUnspecified):
		// Go can't be handed a block's 0.0.0.0: it would dial it, and that can reach the
		// local machine.
		ttl := negativeTTL(msg)
		if len(addrs) > 0 {
			ttl = min(addrTTL, maxNegativeTTL)
		}
		return verdict{rcode: dnsmessage.RCodeNameError, ttl: ttl}
	case msg.RCode == dnsmessage.RCodeServerFailure && dnssecFailed:
		// The OS resolver often doesn't validate, so asking it would accept the answer
		// that validation rejected.
		return verdict{rcode: dnsmessage.RCodeServerFailure, ttl: minTTL}
	case msg.RCode == dnsmessage.RCodeNameError:
		return verdict{rcode: dnsmessage.RCodeNameError, ttl: negativeTTL(msg)}
	case msg.RCode != dnsmessage.RCodeSuccess:
		return verdict{useOS: true, ttl: minTTL}
	case slices.ContainsFunc(addrs, isLocal):
		// The name is probably internal, with a public record pointing at the local
		// network, so the OS may well know a better answer.
		return verdict{useOS: true, ttl: addrTTL}
	case len(addrs) == 0:
		return verdict{ttl: negativeTTL(msg)}
	}
	return verdict{addrs: addrs, ttl: addrTTL}
}

// isUnspecified reports whether a is an unspecified address. It unmaps the address first,
// because [netip.Addr.IsUnspecified], unlike the checks in isLocal, is false for
// ::ffff:0.0.0.0.
func isUnspecified(a netip.Addr) bool { return a.Unmap().IsUnspecified() }

func isLocal(a netip.Addr) bool {
	return a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast()
}

// findAddrs follows the CNAME records from q's name and returns the addresses of q's type
// that the last name owns, with how long the answer can be cached: the lowest TTL among
// the records used, at least minTTL. It doesn't rely on the order of the answer section.
// If the chain is longer than maxChain, it returns no addresses.
func findAddrs(msg *dnsmessage.Message, q dnsmessage.Question) ([]netip.Addr, time.Duration) {
	owner, ttl, ok := followChain(msg.Answers, q.Name.String())
	if !ok {
		return nil, minTTL
	}

	var addrs []netip.Addr
	for _, rr := range msg.Answers {
		if len(addrs) == maxAddrs {
			break
		}
		if rr.Header.Type != q.Type || !strings.EqualFold(rr.Header.Name.String(), owner) {
			continue
		}
		switch body := rr.Body.(type) {
		case *dnsmessage.AResource:
			addrs = append(addrs, netip.AddrFrom4(body.A))
		case *dnsmessage.AAAAResource:
			addrs = append(addrs, netip.AddrFrom16(body.AAAA))
		}
		ttl = min(ttl, rr.Header.TTL)
	}
	return addrs, max(time.Duration(ttl)*time.Second, minTTL)
}

// followChain returns the name that CNAME records lead to from name, and the lowest TTL
// among those records. ok is false if there are more than maxChain of them.
func followChain(answers []dnsmessage.Resource, name string) (end string, ttl uint32, ok bool) {
	cnames := make(map[string]dnsmessage.Resource)
	for _, rr := range answers {
		if _, isCNAME := rr.Body.(*dnsmessage.CNAMEResource); isCNAME {
			cnames[strings.ToLower(rr.Header.Name.String())] = rr
		}
	}

	ttl = math.MaxUint32
	for followed := 0; ; followed++ {
		rr, found := cnames[strings.ToLower(name)]
		if !found {
			return name, ttl, true
		}
		if followed == maxChain {
			return "", 0, false
		}
		ttl = min(ttl, rr.Header.TTL)
		name = rr.Body.(*dnsmessage.CNAMEResource).CNAME.String()
	}
}

// extendedErrors reports whether msg carries an Extended DNS Error that marks a block, or
// one that marks a DNSSEC failure. A response can carry several: Google has sent 9 and 7
// together.
func extendedErrors(msg *dnsmessage.Message) (blocked, dnssecFailed bool) {
	for _, rr := range msg.Additionals {
		opt, ok := rr.Body.(*dnsmessage.OPTResource)
		if !ok {
			continue
		}
		for _, o := range opt.Options {
			if o.Code != optionEDE || len(o.Data) < 2 {
				continue
			}
			switch binary.BigEndian.Uint16(o.Data) {
			case 4, 15, 16, 17: // Forged Answer, Blocked, Censored, Filtered
				blocked = true
			case 1, 2, 5, 6, 7, 8, 9, 10, 11, 12: // the DNSSEC codes
				dnssecFailed = true
			}
		}
	}
	return blocked, dnssecFailed
}

// negativeTTL returns how long to cache an answer without addresses: the RFC 2308 rule,
// the lower of the SOA record's TTL and its MINIMUM field, within minTTL and
// maxNegativeTTL. Quad9's blocks come without an SOA, and they get minTTL, so that they
// don't cost a round trip for each new connection.
func negativeTTL(msg *dnsmessage.Message) time.Duration {
	for _, rr := range msg.Authorities {
		if soa, ok := rr.Body.(*dnsmessage.SOAResource); ok {
			ttl := time.Duration(min(rr.Header.TTL, soa.MinTTL)) * time.Second
			return min(max(ttl, minTTL), maxNegativeTTL)
		}
	}
	return minTTL
}

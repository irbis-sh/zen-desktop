package proxy

import (
	"context"
	"net"
	"net/netip"
	"strings"
)

// dial opens every upstream connection. With a resolver, it sends hosts that contain a
// dot and aren't IP addresses to dnsDialer, and everything else to netDialer, which
// uses the OS resolver.
//
// Hosts without a dot, such as "nas" or "localhost", stay with the OS: Go's resolver
// would first try them under each search domain, and on a typical Mac that is "local",
// an mDNS lookup that takes about 5 s to fail.
func (p *Proxy) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if p.dnsDialer == nil {
		return p.netDialer.DialContext(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || !strings.Contains(host, ".") {
		return p.netDialer.DialContext(ctx, network, addr)
	}
	// IP addresses can contain dots too: "192.0.2.1", "::ffff:192.0.2.1", or a zone
	// such as "fe80::1%eth0.100".
	if _, err := netip.ParseAddr(host); err == nil {
		return p.netDialer.DialContext(ctx, network, addr)
	}

	// The trailing dot stops Go from trying the name under the search domains too.
	// Otherwise, after an NXDOMAIN for "malware.example", Go would ask for
	// "malware.example.local.", which the resolver sends to the OS in plaintext. Callers
	// have already taken the TLS server name from addr, so the dot doesn't reach it.
	name := strings.TrimSuffix(host, ".")
	conn, err := p.dnsDialer.DialContext(ctx, network, net.JoinHostPort(name+".", port))
	if err != nil {
		return nil, tidyDNSError(err, name)
	}
	return conn, nil
}

// tidyDNSError makes a lookup error from dnsDialer read as one from the OS resolver
// does. Go's text has the rooted name and a server from the system's DNS configuration,
// which was never asked: the resolver's Dial hook answered instead. This text ends up
// on the error page and in the log.
func tidyDNSError(err error, name string) error {
	opErr, ok := err.(*net.OpError)
	if !ok {
		return err
	}
	dnsErr, ok := opErr.Err.(*net.DNSError)
	if !ok {
		return err
	}
	// Go merges concurrent lookups of a name and gives every caller the same DNSError,
	// so it is copied. The OpError is made for each dial and can be changed.
	cp := *dnsErr
	cp.Name, cp.Server = name, ""
	opErr.Err = &cp
	return opErr
}

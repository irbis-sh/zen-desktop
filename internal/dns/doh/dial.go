package doh

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/netip"
	"slices"
	"time"
)

const (
	// connectTimeout limits one call of dialTLS, lookup and all addresses included.
	// net/http keeps dialling after the query that started the dial gives up, so that a
	// later query can use the connection. The query's time budget therefore doesn't bound
	// the dial, and without this limit a dial on a network that drops packets would run
	// until the OS gives up, 75 s on macOS.
	connectTimeout = 10 * time.Second
	// staggerDelay is how long an attempt runs before the next address gets one too,
	// as in Happy Eyeballs (RFC 8305).
	staggerDelay = 250 * time.Millisecond
)

var errNoAddresses = errors.New("no addresses to connect to")

// dialTLS is the transport's DialTLSContext. It ignores the host in addr, and connects
// to the client's own addresses on addr's port, with the URL's host as the TLS server name.
// If the client has no bootstrap addresses, it looks up the URL's host first. Queries
// that need a connection wait for one call of dialTLS (see MaxConnsPerHost in
// newTransport), and net/http runs it detached from them, so a query that gives up
// doesn't cancel it.
//
// It does the TLS handshake itself, rather than leaving it to net/http, so that a failed
// handshake moves on to the next address. Behind a captive portal, or a router that
// answers on 1.1.1.1 itself, TCP connects but TLS fails.
//
// The stagger covers the handshake too, so a handshake that stalls after TCP connects
// doesn't hold up the other addresses. The cost is on links with a round trip over about
// 125 ms: the next address starts before the first handshake ends, and the extra
// connections are thrown away.
//
// If every address fails, dialTLS reports the error from the first address in the order.
// The others are fallbacks, and their errors can hide the real problem: on a network
// without IPv6, the IPv6 addresses fail at once with "no route to host", while the IPv4
// addresses time out or fail TLS.
func (c *Client) dialTLS(ctx context.Context, _, addr string) (net.Conn, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	addrs, err := c.addrs(ctx)
	if err != nil {
		c.connFailed(ctx, err)
		return nil, err
	}

	type result struct {
		conn  *tls.Conn
		index int
		err   error
	}
	results := make(chan result)
	next, running := 0, 0
	start := func() {
		index := next
		next++
		running++
		go func() {
			conn, err := c.connect(ctx, addrs[index], port)
			results <- result{conn, index, err}
		}()
	}

	start()
	timer := time.NewTimer(staggerDelay)
	defer timer.Stop()
	var firstAddrErr error
	for running > 0 {
		select {
		case <-timer.C:
		case r := <-results:
			running--
			if r.err == nil {
				cancel()
				go func() {
					for range running {
						if r := <-results; r.conn != nil {
							r.conn.Close()
						}
					}
				}()
				c.mu.Lock()
				c.connErr, c.lastWorked = nil, addrs[r.index]
				c.mu.Unlock()
				return r.conn, nil
			}
			if r.index == 0 {
				firstAddrErr = r.err
				c.connFailed(ctx, r.err)
			}
		}
		// The stagger delay passed or an attempt failed: either way, the next address gets a go.
		if next < len(addrs) {
			start()
			timer.Reset(staggerDelay)
		}
	}
	return nil, firstAddrErr
}

// addrs returns the addresses to dial, in order: the one that last worked, then IPv4,
// then IPv6. If the client has no bootstrap addresses, addrs looks them up.
func (c *Client) addrs(ctx context.Context) ([]netip.Addr, error) {
	addrs := c.bootstrap
	if len(addrs) == 0 {
		var err error
		if addrs, err = c.lookup(ctx, c.host); err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, errNoAddresses
		}
	}
	return c.order(addrs), nil
}

// order returns a copy of addrs sorted for dialling: the address that last worked, then
// IPv4, then IPv6.
func (c *Client) order(addrs []netip.Addr) []netip.Addr {
	c.mu.Lock()
	last := c.lastWorked
	c.mu.Unlock()
	rank := func(a netip.Addr) int {
		switch {
		case a == last:
			return 0
		case a.Is4():
			return 1
		default:
			return 2
		}
	}
	ordered := slices.Clone(addrs)
	slices.SortStableFunc(ordered, func(a, b netip.Addr) int { return cmp.Compare(rank(a), rank(b)) })
	return ordered
}

// connect opens a TCP connection to addr and completes a TLS handshake on it.
func (c *Client) connect(ctx context.Context, addr netip.Addr, port string) (*tls.Conn, error) {
	raw, err := c.dial(ctx, "tcp", net.JoinHostPort(addr.String(), port))
	if err != nil {
		return nil, err
	}
	conn := tls.Client(raw, c.tlsConfig)
	if err := conn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	return conn, nil
}

// connFailed records err as the most recent connection error, unless ctx was cancelled:
// "operation was canceled" from Reset or Close would hide the last real error.
func (c *Client) connFailed(ctx context.Context, err error) {
	if errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	c.mu.Lock()
	c.connErr = err
	c.mu.Unlock()
}

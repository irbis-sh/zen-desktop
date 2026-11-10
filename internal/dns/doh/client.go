// Package doh implements a DNS-over-HTTPS (RFC 8484) client.
package doh

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// silenceLimit is how long the client goes without a response before it moves to new
// connections. After sleep, a connection can be dead without the OS knowing, and a query
// sent on it would wait out its whole budget. The HTTP/2 ping doesn't catch this in time:
// it starts only after 15 s without traffic.
const silenceLimit = time.Minute

// Client sends DNS queries to one DoH server. Create one with [NewClient].
// A Client is safe for concurrent use by multiple goroutines.
type Client struct {
	url string
	// urlErr is set if url isn't an https URL with a host. Every query then fails with it.
	urlErr error
	host   string
	// bootstrap holds the addresses to dial. If it is empty, dialTLS looks up host.
	bootstrap []netip.Addr
	tlsConfig *tls.Config
	// transport is replaced by Reset.
	transport atomic.Pointer[http.Transport]

	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// now returns the wall-clock time. Tests replace it.
	now func() time.Time

	mu sync.Mutex
	// lastWorked is the address of the last connection whose TLS handshake completed since
	// the last Reset.
	lastWorked netip.Addr
	// silentSince is when the current stretch without a valid response began: the last
	// response, or the last time the client moved to new connections (Reset, or the
	// silence check).
	silentSince time.Time
	// connErr is the error the last failed dial reported (see dialTLS), or nil if a dial or
	// query has succeeded since.
	connErr error
}

// NewClient returns a client for the DoH server at rawURL. bootstrap holds the server's
// addresses. If it is empty, the client looks up rawURL's host each time it connects. If
// rawURL isn't an https URL with a host, every query fails.
func NewClient(rawURL string, bootstrap []netip.Addr) *Client {
	return newClient(rawURL, bootstrap, (&net.Dialer{}).DialContext, lookupIP, nil)
}

// wallClock returns the time without its monotonic reading. Time asleep has to count
// towards silenceLimit, and the monotonic clock stops during sleep.
func wallClock() time.Time { return time.Now().Round(0) }

func lookupIP(ctx context.Context, host string) ([]netip.Addr, error) {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	for i, a := range addrs {
		addrs[i] = a.Unmap()
	}
	return addrs, err
}

// newClient lets tests replace the network, the OS resolver and the trusted root
// certificates. A nil rootCAs means the system's.
func newClient(
	rawURL string,
	bootstrap []netip.Addr,
	dial func(ctx context.Context, network, addr string) (net.Conn, error),
	lookup func(ctx context.Context, host string) ([]netip.Addr, error),
	rootCAs *x509.CertPool,
) *Client {
	var host string
	u, urlErr := url.Parse(rawURL)
	if urlErr == nil {
		host = u.Hostname()
		if u.Scheme != "https" || host == "" {
			urlErr = fmt.Errorf("URL %q isn't an https URL with a host", rawURL)
		}
	}
	if ip, err := netip.ParseAddr(host); err == nil && len(bootstrap) == 0 {
		bootstrap = []netip.Addr{ip.Unmap()}
	}

	c := &Client{
		url:       rawURL,
		urlErr:    urlErr,
		host:      host,
		bootstrap: bootstrap,
		tlsConfig: &tls.Config{
			ServerName: host,
			NextProtos: []string{"h2", "http/1.1"},
			RootCAs:    rootCAs,
			MinVersion: tls.VersionTLS12,
		},
		dial:        dial,
		lookup:      lookup,
		now:         wallClock,
		silentSince: wallClock(),
	}
	c.transport.Store(c.newTransport())
	return c
}

func (c *Client) newTransport() *http.Transport {
	return &http.Transport{
		// Proxy is nil, as in any new Transport: the client must not go through
		// the system proxy, which is Zen itself.
		DialTLSContext: c.dialTLS,
		// A custom dialler turns off automatic HTTP/2.
		ForceAttemptHTTP2: true,
		// Otherwise net/http asks for gzip. When a server pads its answer to a padded query,
		// gzip would squeeze out the zero bytes, and the answer's size would give away its
		// length again.
		DisableCompression: true,
		HTTP2: &http.HTTP2Config{
			SendPingTimeout: 15 * time.Second,
			PingTimeout:     5 * time.Second,
		},
		// This also closes the connections of a transport that Reset or the silence
		// check replaced, once their queries finish. Nothing else closes them.
		IdleConnTimeout: 90 * time.Second,
		// Without a limit, each query that finds no connection dials its own, and a page
		// load after start, Reset or the silence check opens dozens of connections only
		// to keep one. An HTTP/2 connection that can't take more streams doesn't count
		// against the limit. The cost falls on a server that only speaks HTTP/1.1, which
		// then answers one query at a time.
		MaxConnsPerHost: 1,
	}
}

// Query sends one question and returns the answer. The question's name must be fully
// qualified. If ctx's deadline passes and the client's most recent connection attempt
// failed, Query returns that attempt's error, such as a TLS error, rather than
// [context.DeadlineExceeded].
func (c *Client) Query(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
	if c.urlErr != nil {
		return nil, c.urlErr
	}
	query, err := packQuery(q)
	if err != nil {
		return nil, fmt.Errorf("pack query: %w", err)
	}
	c.renewIfSilent()

	msg, err := c.exchange(ctx, q, query)
	if err != nil && ctx.Err() == nil {
		// The query failed before its time ran out, as with a reset or a GOAWAY that
		// net/http doesn't retry itself. HTTP/2 drops a broken connection from its pool,
		// so this attempt gets a new one.
		msg, err = c.exchange(ctx, q, query)
	}
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		c.mu.Lock()
		if c.connErr != nil {
			err = c.connErr
		}
		c.mu.Unlock()
	}
	return msg, err
}

// exchange sends one query over HTTP.
func (c *Client) exchange(ctx context.Context, q dnsmessage.Question, query []byte) (*dnsmessage.Message, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mediaType)
	req.Header.Set("Accept", mediaType)

	resp, err := c.transport.Load().RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMessageSize+1))
	if err != nil {
		return nil, err
	}
	msg, err := parseResponse(resp, body, q)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.silentSince = c.now()
	// A dial that no query waits for any more can fail after a later one succeeded.
	// Without this, its error would be reported for every timeout until the next dial.
	c.connErr = nil
	c.mu.Unlock()
	return msg, nil
}

// Reset makes later queries use new connections. Queries already running finish or time
// out on their current connections.
func (c *Client) Reset() {
	c.mu.Lock()
	c.silentSince = c.now()
	c.connErr = nil
	// The address that worked on the old network says nothing about the new one. If it was
	// IPv6 and the new network has no IPv6, it would fail at once, and dialTLS would report
	// that error rather than the IPv4 addresses' errors.
	c.lastWorked = netip.Addr{}
	// CloseIdleConnections leaves busy connections open, and HTTP/2 sends new queries on
	// them too. Without a new transport, queries after a network change or sleep would
	// keep landing on a busy connection that may be dead.
	old := c.transport.Swap(c.newTransport())
	c.mu.Unlock()
	// This also cancels dials that no query is waiting for.
	old.CloseIdleConnections()
}

// Close closes the client's idle connections. Queries already running finish or time out.
func (c *Client) Close() {
	c.transport.Load().CloseIdleConnections()
}

// renewIfSilent moves to new connections, as Reset does, if no response has come for
// silenceLimit. Unlike Reset, it keeps connErr: during an outage, the new dial can outlast
// the query, which should still report the last real error.
func (c *Client) renewIfSilent() {
	c.mu.Lock()
	now := c.now()
	// A negative stretch means the wall clock stepped back. It counts as silence, or the
	// check would wait out the step as well.
	if d := now.Sub(c.silentSince); d >= 0 && d <= silenceLimit {
		c.mu.Unlock()
		return
	}
	// Restart the stretch, so that during an outage each query doesn't cancel the dials
	// earlier queries started.
	c.silentSince = now
	// The swap happens under mu, so a query that checks for silence at the same time
	// waits, then gets the new transport.
	old := c.transport.Swap(c.newTransport())
	c.mu.Unlock()
	old.CloseIdleConnections()
}

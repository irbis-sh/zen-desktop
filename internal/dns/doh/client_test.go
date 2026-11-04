package doh

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

var (
	addr1 = netip.MustParseAddr("192.0.2.1")
	addr2 = netip.MustParseAddr("192.0.2.2")
)

func TestQuerySendsPaddedPOST(t *testing.T) {
	t.Parallel()

	var got *http.Request
	var body []byte
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request, q dnsmessage.Message) {
		got = r
		body, _ = io.ReadAll(r.Body)
		writeAnswer(w, q)
	})
	c, _ := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})

	if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
		t.Fatalf("Query: %v", err)
	}

	if got.Method != http.MethodPost || got.ProtoMajor != 2 {
		t.Errorf("request is %s over HTTP/%d, want POST over HTTP/2", got.Method, got.ProtoMajor)
	}
	for _, h := range []string{"Content-Type", "Accept"} {
		if v := got.Header.Get(h); v != mediaType {
			t.Errorf("%s = %q, want %q", h, v, mediaType)
		}
	}
	if v := got.Header.Get("Accept-Encoding"); v != "" {
		t.Errorf("Accept-Encoding = %q, want none", v)
	}
	if len(body)%paddingBlock != 0 {
		t.Errorf("query is %d bytes, want a multiple of %d", len(body), paddingBlock)
	}

	var msg dnsmessage.Message
	if err := msg.Unpack(body); err != nil {
		t.Fatal(err)
	}
	if msg.ID != 0 || !msg.RecursionDesired {
		t.Errorf("ID = %d, RD = %t, want 0 and true", msg.ID, msg.RecursionDesired)
	}
	if len(msg.Additionals) != 1 {
		t.Fatalf("query has %d additional records, want the OPT record only", len(msg.Additionals))
	}
	opt, ok := msg.Additionals[0].Body.(*dnsmessage.OPTResource)
	if !ok || len(opt.Options) != 1 || opt.Options[0].Code != optionPadding {
		t.Errorf("OPT record = %+v, want only the Padding option", msg.Additionals[0].Body)
	}
}

func TestQueryRejectsBadResponses(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		respond func(w http.ResponseWriter, q dnsmessage.Message)
		// want is part of the error, so that each case fails only the check it is for.
		want string
	}{
		"status": {func(w http.ResponseWriter, q dnsmessage.Message) {
			a := answerFor(q)
			b, _ := a.Pack()
			w.Header().Set("Content-Type", mediaType)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(b)
		}, "status"},
		"content type": {func(w http.ResponseWriter, q dnsmessage.Message) {
			a := answerFor(q)
			b, _ := a.Pack()
			w.Header().Set("Content-Type", "text/html")
			w.Write(b)
		}, "content type"},
		"ID": {func(w http.ResponseWriter, q dnsmessage.Message) {
			a := answerFor(q)
			a.ID = 1
			writeMessage(w, a)
		}, "isn't an answer"},
		"question": {func(w http.ResponseWriter, q dnsmessage.Message) {
			a := answerFor(q)
			a.Questions[0].Name = dnsmessage.MustNewName("other.example.")
			writeMessage(w, a)
		}, "different question"},
		"question type": {func(w http.ResponseWriter, q dnsmessage.Message) {
			a := answerFor(q)
			a.Questions[0].Type = dnsmessage.TypeAAAA
			writeMessage(w, a)
		}, "different question"},
		"question class": {func(w http.ResponseWriter, q dnsmessage.Message) {
			a := answerFor(q)
			a.Questions[0].Class = dnsmessage.ClassCHAOS
			writeMessage(w, a)
		}, "different question"},
		"not a response": {func(w http.ResponseWriter, q dnsmessage.Message) {
			writeMessage(w, q)
		}, "isn't an answer"},
		"too large": {func(w http.ResponseWriter, _ dnsmessage.Message) {
			w.Header().Set("Content-Type", mediaType)
			w.Write(make([]byte, maxMessageSize+1))
		}, "larger than"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request, q dnsmessage.Message) {
				if q.Questions[0].Name.String() == "bad.example." {
					tt.respond(w, q)
					return
				}
				writeAnswer(w, q)
			})
			c, n := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})

			if _, err := c.Query(t.Context(), question("bad.example.")); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one containing %q", err, tt.want)
			}
			if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
				t.Fatalf("Query after the bad response: %v", err)
			}
			if d := n.dials.Load(); d != 1 {
				t.Errorf("%d dials, want 1: the connection should be kept", d)
			}
		})
	}
}

func TestQueryAcceptsContentTypeParameters(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request, q dnsmessage.Message) {
		a := answerFor(q)
		b, _ := a.Pack()
		w.Header().Set("Content-Type", "application/dns-message; charset=binary")
		w.Write(b)
	})
	c, _ := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})

	if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
		t.Fatalf("Query: %v", err)
	}
}

func TestQueryComparesQuestionNameCaseInsensitively(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request, q dnsmessage.Message) {
		a := answerFor(q)
		a.Questions[0].Name = dnsmessage.MustNewName("OK.Example.")
		writeMessage(w, a)
	})
	c, _ := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})

	if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
		t.Fatalf("Query: %v", err)
	}
}

func TestQueryMovesToNextAddress(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		first behaviour
		// minGap and maxGap bound the time between the two dials.
		minGap, maxGap time.Duration
	}{
		// A dial that fails at once starts the next address without waiting for staggerDelay.
		"refused":    {refuse, 0, staggerDelay / 2},
		"TLS failed": {failTLS, 0, staggerDelay / 2},
		// The first dial records its start a moment after the stagger timer starts.
		"dropped": {hang, staggerDelay / 2, 2 * staggerDelay},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t, answerAll)
			c, n := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: tt.first, addr2: connect})

			if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
				t.Fatalf("Query: %v", err)
			}
			if gap := n.dialStart(addr2).Sub(n.dialStart(addr1)); gap < tt.minGap || gap > tt.maxGap {
				t.Errorf("second dial started %s after the first, want between %s and %s", gap, tt.minGap, tt.maxGap)
			}
			if got := c.order([]netip.Addr{addr1, addr2})[0]; got != addr2 {
				t.Errorf("first address after the query = %s, want %s, which last worked", got, addr2)
			}
			c.Reset()
			if got := c.order([]netip.Addr{addr1, addr2})[0]; got != addr1 {
				t.Errorf("first address after Reset = %s, want %s", got, addr1)
			}
		})
	}
}

func TestConcurrentQueriesShareOneDial(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerAll)
	c, n := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
				t.Errorf("Query: %v", err)
			}
		})
	}
	wg.Wait()
	if d := n.dials.Load(); d != 1 {
		t.Errorf("%d dials, want 1", d)
	}
}

func TestQueryRetriesAfterConnectionDrops(t *testing.T) {
	t.Parallel()

	var dropped atomic.Bool
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request, q dnsmessage.Message) {
		if !dropped.Swap(true) {
			r.Context().Value(connKey{}).(net.Conn).Close()
			return
		}
		writeAnswer(w, q)
	})
	c, n := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})

	if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if d := n.dials.Load(); d != 2 {
		t.Errorf("%d dials, want 2", d)
	}
}

func TestQueryTimeoutReportsLastConnectionError(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerAll)
	c, _ := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect, addr2: hang})
	// Trust nothing, so the handshake with addr1 fails.
	c.tlsConfig.RootCAs = x509.NewCertPool()

	ctx, cancel := context.WithTimeout(t.Context(), 2*staggerDelay)
	defer cancel()
	_, err := c.Query(ctx, question("ok.example."))
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("error = %v, want the TLS error", err)
	}
}

func TestQueryTimeoutIgnoresLaterAddressErrors(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerAll)
	// addr2 stands in for an IPv6 address on a network without IPv6.
	c, _ := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: hang, addr2: refuse})

	ctx, cancel := context.WithTimeout(t.Context(), 2*staggerDelay)
	defer cancel()
	_, err := c.Query(ctx, question("ok.example."))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestQueryTimeoutAfterSuccessReportsDeadline(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerOrHang)
	// The handshake with addr1 fails, and the one with addr2 succeeds.
	c, _ := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: failTLS, addr2: connect})
	if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
		t.Fatalf("Query: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := c.Query(ctx, question("hang.example."))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestQueryOpensNewConnectionAfterSilence(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerAll)
	c, n := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})
	now := time.Now()
	c.now = func() time.Time { return now }

	query := func(after time.Duration) {
		t.Helper()
		now = now.Add(after)
		if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
			t.Fatalf("Query: %v", err)
		}
	}
	query(0)
	query(silenceLimit - time.Second)
	if d := n.dials.Load(); d != 1 {
		t.Errorf("%d dials before the silence limit, want 1", d)
	}
	query(silenceLimit + time.Second)
	if d := n.dials.Load(); d != 2 {
		t.Errorf("%d dials after the silence limit, want 2", d)
	}
	query(-time.Hour)
	if d := n.dials.Load(); d != 3 {
		t.Errorf("%d dials after the clock stepped back, want 3", d)
	}
}

func TestResetOpensNewConnection(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerAll)
	c, n := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})

	for range 2 {
		if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
			t.Fatalf("Query: %v", err)
		}
		c.Reset()
	}
	if d := n.dials.Load(); d != 2 {
		t.Errorf("%d dials, want 2", d)
	}
}

func TestNewConnectionWhileBusy(t *testing.T) {
	t.Parallel()

	// Each case runs with a query hanging on the only connection. Without a new transport,
	// the next query would share that busy connection.
	tests := map[string]func(c *Client, sleep func(time.Duration)){
		"Reset":   func(c *Client, _ func(time.Duration)) { c.Reset() },
		"silence": func(_ *Client, sleep func(time.Duration)) { sleep(silenceLimit + time.Second) },
	}

	for name, renew := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			arrived := make(chan struct{})
			srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request, q dnsmessage.Message) {
				if q.Questions[0].Name.String() == "hang.example." {
					close(arrived)
				}
				answerOrHang(w, r, q)
			})
			c, n := newTestClient(t, srv, map[netip.Addr]behaviour{addr1: connect})
			var slept atomic.Int64
			c.now = func() time.Time { return time.Now().Add(time.Duration(slept.Load())) }
			if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
				t.Fatalf("Query: %v", err)
			}

			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() {
				c.Query(ctx, question("hang.example."))
				close(done)
			}()
			t.Cleanup(func() {
				cancel()
				<-done
			})
			<-arrived

			renew(c, func(d time.Duration) { slept.Add(int64(d)) })
			if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
				t.Fatalf("Query after %s: %v", name, err)
			}
			if d := n.dials.Load(); d != 2 {
				t.Errorf("%d dials, want 2", d)
			}
			select {
			case <-done:
				t.Errorf("%s ended the running query", name)
			default:
			}
		})
	}
}

func TestIPHostSkipsLookup(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerAll)
	ip := netip.MustParseAddr("127.0.0.1")
	n := newTestNetwork(srv, map[netip.Addr]behaviour{ip: connect})
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		t.Error("lookup called for an IP host")
		return nil, errors.New("unexpected lookup")
	}
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	c := newClient("https://127.0.0.1:"+port+"/dns-query", nil, n.dial, lookup, rootCAs(srv))
	t.Cleanup(c.Close)

	if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
		t.Fatalf("Query: %v", err)
	}
	c.Reset()
}

func TestInvalidURLFailsQueries(t *testing.T) {
	t.Parallel()

	dial := func(context.Context, string, string) (net.Conn, error) {
		t.Error("dial called for an invalid URL")
		return nil, errors.New("unexpected dial")
	}
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		t.Error("lookup called for an invalid URL")
		return nil, errors.New("unexpected lookup")
	}
	for _, u := range []string{"://", "http://example.com/dns-query", "https:///dns-query"} {
		c := newClient(u, []netip.Addr{addr1}, dial, lookup, nil)
		if _, err := c.Query(t.Context(), question("ok.example.")); err == nil {
			t.Errorf("Query with URL %q succeeded", u)
		}
		c.Reset()
		c.Close()
	}
}

func TestQueryLooksUpCustomHost(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, answerAll)
	n := newTestNetwork(srv, map[netip.Addr]behaviour{addr1: connect})
	var lookups atomic.Int32
	var networkUp atomic.Bool
	lookup := func(_ context.Context, host string) ([]netip.Addr, error) {
		lookups.Add(1)
		if host != "example.com" {
			t.Errorf("looked up %q, want example.com", host)
		}
		if !networkUp.Load() {
			return nil, errors.New("network is down")
		}
		return []netip.Addr{addr1}, nil
	}
	c := newClient(serverURL(srv), nil, n.dial, lookup, rootCAs(srv))
	t.Cleanup(c.Close)
	if l := lookups.Load(); l != 0 {
		t.Errorf("%d lookups before the first query, want 0", l)
	}

	if _, err := c.Query(t.Context(), question("ok.example.")); err == nil {
		t.Fatal("Query succeeded with the lookup failing")
	}
	failed := lookups.Load()
	networkUp.Store(true)
	for range 2 {
		if _, err := c.Query(t.Context(), question("ok.example.")); err != nil {
			t.Fatalf("Query after the failed lookup: %v", err)
		}
	}
	// The second successful query reuses the connection, so it needs no lookup.
	if l := lookups.Load() - failed; l != 1 {
		t.Errorf("%d lookups after the network came up, want 1", l)
	}
}

func TestAddrsOrder(t *testing.T) {
	t.Parallel()

	v4a, v4b := netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")
	v6a, v6b := netip.MustParseAddr("2001:db8::1"), netip.MustParseAddr("2001:db8::2")
	c := &Client{lastWorked: v6b}

	got := c.order([]netip.Addr{v6a, v4a, v6b, v4b})
	want := []netip.Addr{v6b, v4a, v4b, v6a}
	if !slices.Equal(got, want) {
		t.Errorf("order() = %v, want %v", got, want)
	}
}

type behaviour int

const (
	connect behaviour = iota
	refuse
	hang
	// failTLS connects, but the TLS handshake fails at once, as behind a captive portal.
	failTLS
)

// testNetwork maps made-up addresses to behaviours. Addresses that connect reach the test
// server.
type testNetwork struct {
	server     string
	behaviours map[netip.Addr]behaviour
	// dials counts the connections made to the test server.
	dials atomic.Int32

	mu         sync.Mutex
	dialStarts map[netip.Addr]time.Time
}

func newTestNetwork(srv *httptest.Server, behaviours map[netip.Addr]behaviour) *testNetwork {
	return &testNetwork{
		server:     srv.Listener.Addr().String(),
		behaviours: behaviours,
		dialStarts: make(map[netip.Addr]time.Time),
	}
}

// dialStart returns when addr was first dialled.
func (n *testNetwork) dialStart(addr netip.Addr) time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.dialStarts[addr]
}

func (n *testNetwork) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	ap := netip.MustParseAddrPort(addr)
	n.mu.Lock()
	if _, ok := n.dialStarts[ap.Addr()]; !ok {
		n.dialStarts[ap.Addr()] = time.Now()
	}
	n.mu.Unlock()

	switch n.behaviours[ap.Addr()] {
	case refuse:
		return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.ECONNREFUSED}
	case hang:
		<-ctx.Done()
		return nil, ctx.Err()
	case failTLS:
		conn, peer := net.Pipe()
		peer.Close()
		return conn, nil
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, n.server)
	if err == nil {
		n.dials.Add(1)
	}
	return conn, err
}

type connKey struct{}

// newTestServer starts an HTTP/2 DoH server whose certificate is valid for example.com.
func newTestServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, q dnsmessage.Message)) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var q dnsmessage.Message
		if err := q.Unpack(body); err != nil || len(q.Questions) != 1 {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		handle(w, r, q)
	}))
	srv.EnableHTTP2 = true
	srv.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, connKey{}, c)
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// newTestClient returns a client whose bootstrap addresses are those in behaviours, in
// ascending order.
func newTestClient(t *testing.T, srv *httptest.Server, behaviours map[netip.Addr]behaviour) (*Client, *testNetwork) {
	t.Helper()
	n := newTestNetwork(srv, behaviours)
	var bootstrap []netip.Addr
	for _, a := range []netip.Addr{addr1, addr2} {
		if _, ok := behaviours[a]; ok {
			bootstrap = append(bootstrap, a)
		}
	}
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		t.Error("lookup called with bootstrap addresses")
		return nil, errors.New("unexpected lookup")
	}
	c := newClient(serverURL(srv), bootstrap, n.dial, lookup, rootCAs(srv))
	t.Cleanup(c.Close)
	return c, n
}

func serverURL(srv *httptest.Server) string {
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	return "https://example.com:" + port + "/dns-query"
}

func rootCAs(srv *httptest.Server) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return pool
}

func answerAll(w http.ResponseWriter, _ *http.Request, q dnsmessage.Message) {
	writeAnswer(w, q)
}

// answerOrHang never answers hang.example., and answers everything else.
func answerOrHang(w http.ResponseWriter, r *http.Request, q dnsmessage.Message) {
	if q.Questions[0].Name.String() == "hang.example." {
		<-r.Context().Done()
		return
	}
	writeAnswer(w, q)
}

func answerFor(q dnsmessage.Message) dnsmessage.Message {
	return dnsmessage.Message{
		Header:    dnsmessage.Header{ID: q.ID, Response: true, RecursionDesired: true, RecursionAvailable: true},
		Questions: q.Questions,
		Answers: []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{Name: q.Questions[0].Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
			Body:   &dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}},
		}},
	}
}

func writeAnswer(w http.ResponseWriter, q dnsmessage.Message) {
	writeMessage(w, answerFor(q))
}

func writeMessage(w http.ResponseWriter, m dnsmessage.Message) {
	b, err := m.Pack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Write(b)
}

func question(name string) dnsmessage.Question {
	return dnsmessage.Question{Name: dnsmessage.MustNewName(name), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}
}

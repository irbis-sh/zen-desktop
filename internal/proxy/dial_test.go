package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestDialSendsDottedNamesToResolver(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	tests := []struct {
		host string
		// asked is the name the resolver should see, for A and AAAA. Empty means the
		// resolver shouldn't be asked at all.
		asked string
	}{
		{"news.example", "news.example."},
		{"news.example.", "news.example."},
		{"localhost", ""},
		{"127.0.0.1", ""},
		{"::ffff:127.0.0.1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Parallel()

			dns := &fakeDNS{}
			p := newTestProxy(t, dns.resolver())

			conn, err := p.dial(t.Context(), "tcp", net.JoinHostPort(tt.host, port))
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			conn.Close()

			var want []string
			if tt.asked != "" {
				want = []string{"A " + tt.asked, "AAAA " + tt.asked}
			}
			if got := dns.asked(); !slices.Equal(got, want) {
				t.Errorf("resolver was asked %q, want %q", got, want)
			}
		})
	}
}

// TestDialSkipsSearchDomains checks that a name the resolver doesn't know isn't asked for
// again under the system's search domains. Without a search list in the system's DNS
// configuration, as on some CI runners, Go has nothing more to ask, so the test can't
// fail there.
func TestDialSkipsSearchDomains(t *testing.T) {
	t.Parallel()

	dns := &fakeDNS{rcode: dnsmessage.RCodeNameError}
	p := newTestProxy(t, dns.resolver())

	if _, err := p.dial(t.Context(), "tcp", "gone.example:80"); err == nil {
		t.Fatal("dial succeeded, want an error")
	}
	if got, want := dns.asked(), []string{"A gone.example.", "AAAA gone.example."}; !slices.Equal(got, want) {
		t.Errorf("resolver was asked %q, want %q", got, want)
	}
}

// TestDialTidiesDNSErrors checks that a failed lookup reads as it does through the OS
// resolver, without the trailing dot dial adds and without the DNS server Go names but
// never asked.
func TestDialTidiesDNSErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rcode    dnsmessage.RCode
		host     string
		want     string
		notFound bool
	}{
		{dnsmessage.RCodeNameError, "gone.example", "dial tcp: lookup gone.example: no such host", true},
		{dnsmessage.RCodeServerFailure, "broken.example", "dial tcp: lookup broken.example: server misbehaving", false},
	}
	for _, tt := range tests {
		t.Run(tt.rcode.String(), func(t *testing.T) {
			t.Parallel()

			p := newTestProxy(t, (&fakeDNS{rcode: tt.rcode}).resolver())

			_, err := p.dial(t.Context(), "tcp", tt.host+":80")
			if err == nil {
				t.Fatal("dial succeeded, want an error")
			}
			if err.Error() != tt.want {
				t.Errorf("error = %q, want %q", err, tt.want)
			}
			var dnsErr *net.DNSError
			if !errors.As(err, &dnsErr) {
				t.Fatalf("error %q has no *net.DNSError", err)
			}
			if dnsErr.IsNotFound != tt.notFound {
				t.Errorf("IsNotFound = %v, want %v", dnsErr.IsNotFound, tt.notFound)
			}
			if dnsErr.Server != "" {
				t.Errorf("Server = %q, want empty", dnsErr.Server)
			}
		})
	}
}

// TestDialCopiesSharedDNSError dials one failing name from several goroutines at once.
// Go merges the lookups and hands each dial the same DNSError, so changing it in place
// is a data race. The text comes out the same either way, so only -race catches it.
func TestDialCopiesSharedDNSError(t *testing.T) {
	t.Parallel()

	// The delay keeps the first lookup open while the others start, so they merge.
	p := newTestProxy(t, (&fakeDNS{rcode: dnsmessage.RCodeNameError, delay: 50 * time.Millisecond}).resolver())

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := p.dial(t.Context(), "tcp", "gone.example:80")
			if want := "dial tcp: lookup gone.example: no such host"; err == nil || err.Error() != want {
				t.Errorf("error = %v, want %q", err, want)
			}
		})
	}
	wg.Wait()
}

// TestUpstreamPathsUseResolver sends each kind of upstream connection to a host that only
// the test's resolver knows. A path that dials without p.dial asks the OS instead, which
// fails with "no such host" here, and in use would send the lookup out in plaintext.
func TestUpstreamPathsUseResolver(t *testing.T) {
	t.Parallel()

	var (
		sniMu sync.Mutex
		sni   string
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWS(r) {
			io.WriteString(w, "ok")
			return
		}
		if r.TLS != nil {
			sniMu.Lock()
			sni = r.TLS.ServerName
			sniMu.Unlock()
		}
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		io.Copy(conn, brw)
	})
	plainSrv := httptest.NewServer(handler)
	t.Cleanup(plainSrv.Close)
	tlsSrv := httptest.NewTLSServer(handler)
	t.Cleanup(tlsSrv.Close)
	_, plainPort, _ := net.SplitHostPort(plainSrv.Listener.Addr().String())
	_, tlsPort, _ := net.SplitHostPort(tlsSrv.Listener.Addr().String())

	// The intercepted rows need a name that httptest's certificate covers: example.com
	// and its subdomains. example.com itself is a real site, which a regression would
	// reach.
	const tlsHost = "upstream.example.com"

	p := newTestProxy(t, (&fakeDNS{}).resolver())
	p.certGenerator = selfSignedCertGenerator{}
	p.requestTransport.TLSClientConfig = &tls.Config{RootCAs: certPoolOf(tlsSrv)}
	p.transparentHosts = []string{"tunnel.example"}
	proxyAddr := serveTestProxy(t, p)

	t.Run("plain request", func(t *testing.T) {
		t.Parallel()

		resp, err := proxyClient(t, proxyAddr).Get("http://plain.example:" + plainPort + "/")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("tunnel", func(t *testing.T) {
		t.Parallel()

		// tunnel answers 200 only once p.dial has connected.
		_, _, resp := connectThrough(t, proxyAddr, "tunnel.example:"+plainPort)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("CONNECT status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("intercepted request", func(t *testing.T) {
		t.Parallel()

		if status, _ := mitmGet(t, proxyAddr, tlsHost+":"+tlsPort, tlsHost); status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
	})

	t.Run("plain websocket", func(t *testing.T) {
		t.Parallel()

		conn, err := net.DialTimeout("tcp", proxyAddr, backstopTimeout)
		if err != nil {
			t.Fatalf("dial proxy: %v", err)
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(backstopTimeout)); err != nil {
			t.Fatalf("set deadline: %v", err)
		}
		websocketEcho(t, conn, "http://ws.example:"+plainPort+"/", true)
	})

	t.Run("intercepted websocket", func(t *testing.T) {
		t.Parallel()

		conn, _, resp := connectThrough(t, proxyAddr, tlsHost+":"+tlsPort)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("CONNECT status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         tlsHost,
			InsecureSkipVerify: true, // #nosec G402 -- the MITM certificate is self-signed on purpose; trust is not under test.
		})
		websocketEcho(t, tlsConn, "https://"+tlsHost+"/", false)

		sniMu.Lock()
		defer sniMu.Unlock()
		if sni != tlsHost {
			t.Errorf("upstream saw server name %q, want %q", sni, tlsHost)
		}
	})
}

// websocketEcho sends a WebSocket upgrade for target on conn, through a proxy if
// viaProxy is set, and checks that the upstream echoes what follows the 101.
func websocketEcho(t *testing.T, conn io.ReadWriter, target string, viaProxy bool) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	write := req.Write
	if viaProxy {
		write = req.WriteProxy
	}
	if err := write(conn); err != nil {
		t.Fatalf("write request: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}

	const msg = "ping"
	if _, err := io.WriteString(conn, msg); err != nil {
		t.Fatalf("write message: %v", err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(br, got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(got) != msg {
		t.Fatalf("echo = %q, want %q", got, msg)
	}
}

// fakeDNS answers Go's DNS queries without a network. It answers A questions with
// 127.0.0.1 and AAAA questions with no records, or every question with rcode if set.
type fakeDNS struct {
	rcode dnsmessage.RCode
	// delay holds each answer back.
	delay time.Duration

	mu        sync.Mutex
	questions []string
}

// resolver returns a resolver that sends its queries to d.
func (d *fakeDNS) resolver() *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			go d.serve(server)
			return client, nil
		},
	}
}

// asked returns the questions d has seen, such as "A news.example.", sorted.
func (d *fakeDNS) asked() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Sorted(slices.Values(d.questions))
}

// serve answers one query on conn. A net.Pipe isn't a PacketConn, so Go frames
// messages as over TCP: a 2-byte length, then the message.
func (d *fakeDNS) serve(conn net.Conn) {
	defer conn.Close()

	var length [2]byte
	if _, err := io.ReadFull(conn, length[:]); err != nil {
		return
	}
	query := make([]byte, binary.BigEndian.Uint16(length[:]))
	if _, err := io.ReadFull(conn, query); err != nil {
		return
	}
	var msg dnsmessage.Message
	if err := msg.Unpack(query); err != nil || len(msg.Questions) != 1 {
		return
	}
	q := msg.Questions[0]
	d.mu.Lock()
	d.questions = append(d.questions, strings.TrimPrefix(q.Type.String(), "Type")+" "+q.Name.String())
	d.mu.Unlock()

	time.Sleep(d.delay)

	answer := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 msg.ID,
			Response:           true,
			RecursionDesired:   msg.RecursionDesired,
			RecursionAvailable: true,
			RCode:              d.rcode,
		},
		Questions: msg.Questions,
	}
	if d.rcode == dnsmessage.RCodeSuccess && q.Type == dnsmessage.TypeA {
		answer.Answers = []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
			Body:   &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
		}}
	}
	packed, err := answer.Pack()
	if err != nil {
		return
	}
	length = [2]byte{}
	binary.BigEndian.PutUint16(length[:], uint16(len(packed))) // #nosec G115 -- an answer with one record is far below 64 KiB
	conn.Write(append(length[:], packed...))
}

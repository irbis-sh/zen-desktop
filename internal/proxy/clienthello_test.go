package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync"
	"testing"

	utls "github.com/refraction-networking/utls"
)

// TestProxyMirrorsClientHello runs the whole path through the proxy: the upstream server
// must see the client's hello. The second connection offers to resume the client's
// session with Zen, and that ticket must not reach upstream.
func TestProxyMirrorsClientHello(t *testing.T) {
	t.Parallel()

	srv, hellos := startHelloRecordingServer(t)
	proxyAddr := startMirroringTestProxy(t, srv)

	// Offering only http/1.1 tells the client's hello apart from Go's, which Zen falls
	// back to and which offers h2 as well. A session cache adds the session ticket
	// extensions, so the direct hello needs one too.
	direct, err := tls.Dial("tcp", srv.Listener.Addr().String(), &tls.Config{
		ServerName:         "example.com",
		RootCAs:            certPoolOf(srv),
		NextProtos:         []string{"http/1.1"},
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
	})
	if err != nil {
		t.Fatalf("dial directly: %v", err)
	}
	direct.Close()

	sessions := tls.NewLRUClientSessionCache(1)
	clientConfig := &tls.Config{
		ServerName:         "example.com",
		NextProtos:         []string{"http/1.1"},
		ClientSessionCache: sessions,
		InsecureSkipVerify: true, // #nosec G402 -- the MITM certificate is self-signed on purpose; trust is not under test.
	}
	getThroughProxy(t, proxyAddr, clientConfig)
	if _, ok := sessions.Get("example.com"); !ok {
		t.Fatal("client kept no session, so the second connection cannot offer one")
	}
	getThroughProxy(t, proxyAddr, clientConfig)

	got := hellos()
	if len(got) != 3 {
		t.Fatalf("server saw %d hellos, want 3", len(got))
	}
	for i, hello := range got[1:] {
		if !reflect.DeepEqual(got[0], hello) {
			t.Errorf("hello on connection %d differs from the client's:\nclient: %+v\nproxy:  %+v", i+1, got[0], hello)
		}
	}
}

// TestMirroredHelloMatchesClient holds the dialer to its purpose: a server must not be
// able to tell the mirrored hello from the client's own.
func TestMirroredHelloMatchesClient(t *testing.T) {
	t.Parallel()

	srv, hellos := startHelloRecordingServer(t)
	clientConfig := &tls.Config{
		ServerName: "example.com",
		RootCAs:    certPoolOf(srv),
		NextProtos: []string{"h2", "http/1.1"},
	}

	direct, err := tls.Dial("tcp", srv.Listener.Addr().String(), clientConfig)
	if err != nil {
		t.Fatalf("dial directly: %v", err)
	}
	direct.Close()

	mirrored, err := mirroringDialerFor(t, srv, goClientHello(t, clientConfig)).DialTLSContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("dial mirrored: %v", err)
	}
	mirrored.Close()

	got := hellos()
	if len(got) != 2 {
		t.Fatalf("server saw %d hellos, want 2", len(got))
	}
	if !reflect.DeepEqual(got[0], got[1]) {
		t.Errorf("mirrored hello differs from the client's:\nclient:   %+v\nmirrored: %+v", got[0], got[1])
	}
}

// TestMirroredConnSpeaksHTTP2 covers the adapter between uTLS and http.Transport,
// which only speaks HTTP/2 over a connection it recognises as TLS. Without it, the
// transport would speak HTTP/1.1 over a connection that negotiated h2.
func TestMirroredConnSpeaksHTTP2(t *testing.T) {
	t.Parallel()

	srv, _ := startHelloRecordingServer(t)
	dialer := mirroringDialerFor(t, srv, goClientHello(t, &tls.Config{
		ServerName: "example.com",
		NextProtos: []string{"h2", "http/1.1"},
	}))
	transport := &http.Transport{
		DialTLSContext:    dialer.DialTLSContext,
		ForceAttemptHTTP2: true,
	}
	defer transport.CloseIdleConnections()

	resp, err := (&http.Client{Transport: transport, Timeout: backstopTimeout}).Get("https://example.com/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()

	if resp.ProtoMajor != 2 {
		t.Fatalf("proto = %s, want HTTP/2.0", resp.Proto)
	}
}

// TestMirroredHelloKeepsUnknownExtensions covers browsers running ahead of uTLS: an
// extension uTLS has no type for must still reach the server.
func TestMirroredHelloKeepsUnknownExtensions(t *testing.T) {
	t.Parallel()

	// 0xca34 is what Chrome 154 sends for its Trust Anchor IDs experiment.
	const unknownExtension = 0xca34

	srv, hellos := startHelloRecordingServer(t)
	hello := captureClientHello(t, func(conn net.Conn) error {
		spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
		if err != nil {
			return err
		}
		spec.Extensions = append(spec.Extensions, &utls.GenericExtension{Id: unknownExtension, Data: []byte{0}})

		client := utls.UClient(conn, &utls.Config{ServerName: "example.com"}, utls.HelloCustom)
		if err := client.ApplyPreset(&spec); err != nil {
			return err
		}
		return client.Handshake()
	})

	conn, err := mirroringDialerFor(t, srv, hello).DialTLSContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("dial mirrored: %v", err)
	}
	conn.Close()

	got := hellos()
	if len(got) != 1 {
		t.Fatalf("server saw %d hellos, want 1", len(got))
	}
	if !slices.Contains(got[0].Extensions, unknownExtension) {
		t.Fatalf("extensions = %v, want %#x among them", got[0].Extensions, unknownExtension)
	}
}

// TestMirroredHelloSurvivesServerPickingSecondKeyShare covers Firefox, which offers key
// shares for both X25519 and P-256, against a server that picks P-256.
func TestMirroredHelloSurvivesServerPickingSecondKeyShare(t *testing.T) {
	t.Parallel()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = &tls.Config{CurvePreferences: []tls.CurveID{tls.CurveP256}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	hello := captureClientHello(t, func(conn net.Conn) error {
		return utls.UClient(conn, &utls.Config{ServerName: "example.com"}, utls.HelloFirefox_Auto).Handshake()
	})

	dialer := mirroringDialerFor(t, srv, hello)
	conn, err := dialer.DialTLSContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("dial mirrored: %v", err)
	}
	conn.Close()

	if dialer.useGoHello.Load() {
		t.Error("dialer fell back to Go's hello")
	}
}

// TestUnmirrorableHelloFallsBackToGo covers a hello that uTLS parses but cannot
// reproduce. The dial must still succeed, with the hello Zen sends without mirroring.
func TestUnmirrorableHelloFallsBackToGo(t *testing.T) {
	t.Parallel()

	srv, hellos := startHelloRecordingServer(t)
	hello := captureClientHello(t, func(conn net.Conn) error {
		spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
		if err != nil {
			return err
		}
		// uTLS cannot generate a finite-field key, so it cannot replay this share.
		for _, ext := range spec.Extensions {
			if keyShare, ok := ext.(*utls.KeyShareExtension); ok {
				keyShare.KeyShares = append(keyShare.KeyShares, utls.KeyShare{Group: utls.FakeCurveFFDHE2048, Data: make([]byte, 256)})
			}
		}

		client := utls.UClient(conn, &utls.Config{ServerName: "example.com"}, utls.HelloCustom)
		if err := client.ApplyPreset(&spec); err != nil {
			return err
		}
		return client.Handshake()
	})

	direct, err := tls.Dial("tcp", srv.Listener.Addr().String(), &tls.Config{
		ServerName: "example.com",
		RootCAs:    certPoolOf(srv),
		NextProtos: []string{"h2", "http/1.1"},
	})
	if err != nil {
		t.Fatalf("dial with Go's hello: %v", err)
	}
	direct.Close()

	dialer := mirroringDialerFor(t, srv, hello)
	conn, err := dialer.DialTLSContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("dial mirrored: %v", err)
	}
	conn.Close()

	got := hellos()
	if len(got) != 2 {
		t.Fatalf("server saw %d hellos, want 2", len(got))
	}
	if !reflect.DeepEqual(got[0], got[1]) {
		t.Errorf("fallback hello differs from Go's:\nGo:       %+v\nfallback: %+v", got[0], got[1])
	}
	if !dialer.useGoHello.Load() {
		t.Error("dialer will try mirroring again on the next dial")
	}
}

// TestUnrelatedFailureKeepsMirroring covers dials that fail for reasons other than the
// hello. They must not switch the client connection to Go's hello for good.
func TestUnrelatedFailureKeepsMirroring(t *testing.T) {
	t.Parallel()

	hello := goClientHello(t, &tls.Config{ServerName: "example.com"})

	tests := []struct {
		name        string
		dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	}{
		{
			name: "dial error",
			dialContext: func(context.Context, string, string) (net.Conn, error) {
				return nil, errors.New("connection refused")
			},
		},
		{
			// Nothing answers on the pipe, so the handshake ends only on the cancelled
			// context.
			name: "handshake cancelled",
			dialContext: func(context.Context, string, string) (net.Conn, error) {
				conn, peer := net.Pipe()
				t.Cleanup(func() { peer.Close() })
				return conn, nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			dialer := &mirroringDialer{dialContext: tt.dialContext, clientHello: hello}
			if conn, err := dialer.DialTLSContext(ctx, "tcp", "example.com:443"); err == nil {
				conn.Close()
				t.Fatal("connected, want an error")
			}
			if dialer.useGoHello.Load() {
				t.Error("dialer gave up on mirroring")
			}
		})
	}
}

// TestMirroredConnKeepsGoFloor covers a client that still offers TLS 1.0. Mirroring its
// hello must not connect upstream below the TLS 1.2 that Go's hello requires.
func TestMirroredConnKeepsGoFloor(t *testing.T) {
	t.Parallel()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS10} // #nosec G402 -- the legacy server is what is under test.
	srv.StartTLS()
	t.Cleanup(srv.Close)

	hello := goClientHello(t, &tls.Config{ServerName: "example.com", MinVersion: tls.VersionTLS10}) // #nosec G402 -- the legacy client is what is under test.

	dialer := mirroringDialerFor(t, srv, hello)
	conn, err := dialer.DialTLSContext(context.Background(), "tcp", "example.com:443")
	if err == nil {
		conn.Close()
		t.Fatal("connected, want an error")
	}
	if !dialer.useGoHello.Load() {
		t.Error("mirrored dial failed for a reason other than the floor")
	}
}

// helloFingerprint is the part of a ClientHello that fingerprinting reads. Key shares
// and the random are left out: they differ on every connection.
type helloFingerprint struct {
	CipherSuites      []uint16
	SupportedCurves   []tls.CurveID
	SupportedPoints   []uint8
	SignatureSchemes  []tls.SignatureScheme
	SupportedProtos   []string
	SupportedVersions []uint16
	Extensions        []uint16
}

// startHelloRecordingServer starts an HTTP/2-capable TLS server with a certificate for
// example.com. The returned function lists the hellos it has seen, in order.
func startHelloRecordingServer(t *testing.T) (*httptest.Server, func() []helloFingerprint) {
	t.Helper()

	var (
		mu     sync.Mutex
		hellos []helloFingerprint
	)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.EnableHTTP2 = true
	srv.TLS = &tls.Config{
		GetConfigForClient: func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			mu.Lock()
			defer mu.Unlock()
			hellos = append(hellos, helloFingerprint{
				CipherSuites:      chi.CipherSuites,
				SupportedCurves:   chi.SupportedCurves,
				SupportedPoints:   chi.SupportedPoints,
				SignatureSchemes:  chi.SignatureSchemes,
				SupportedProtos:   chi.SupportedProtos,
				SupportedVersions: chi.SupportedVersions,
				Extensions:        chi.Extensions,
			})
			return nil, nil
		},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return srv, func() []helloFingerprint {
		mu.Lock()
		defer mu.Unlock()
		return hellos
	}
}

// startMirroringTestProxy starts a proxy that intercepts every CONNECT and sends its
// upstream connections to srv, trusting srv's certificate. It returns the proxy's
// address.
func startMirroringTestProxy(t *testing.T, srv *httptest.Server) string {
	t.Helper()

	return startTestProxy(t, func(p *Proxy) {
		p.certGenerator = selfSignedCertGenerator{}
		p.requestTransport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		}
		p.requestTransport.TLSClientConfig = &tls.Config{RootCAs: certPoolOf(srv)}
	})
}

// getThroughProxy CONNECTs to example.com through the proxy at proxyAddr, completes
// the MITM handshake with config, and checks that GET / succeeds.
func getThroughProxy(t *testing.T, proxyAddr string, config *tls.Config) {
	t.Helper()

	conn, _, resp := connectThrough(t, proxyAddr, "example.com:443")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	tlsConn := tls.Client(conn, config)
	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if err := req.Write(tlsConn); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err = http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// mirroringDialerFor returns a dialer that presents clientHello and connects every
// address to srv.
func mirroringDialerFor(t *testing.T, srv *httptest.Server, clientHello []byte) *mirroringDialer {
	t.Helper()

	return &mirroringDialer{
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
		clientHello: clientHello,
		rootCAs:     certPoolOf(srv),
	}
}

// goClientHello returns the ClientHello record a crypto/tls client with config sends.
func goClientHello(t *testing.T, config *tls.Config) []byte {
	t.Helper()

	return captureClientHello(t, func(conn net.Conn) error {
		return tls.Client(conn, config).Handshake()
	})
}

// captureClientHello returns the ClientHello record that handshake sends on its conn.
func captureClientHello(t *testing.T, handshake func(net.Conn) error) []byte {
	t.Helper()

	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()
	go func() {
		// The handshake cannot complete: nothing answers it. It only has to send the hello.
		handshake(clientSide)
		clientSide.Close()
	}()

	hello, err := readClientHello(serverSide)
	if err != nil {
		t.Fatalf("read client hello: %v", err)
	}
	return hello
}

func certPoolOf(srv *httptest.Server) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return pool
}

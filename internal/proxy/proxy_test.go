package proxy

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/process"
)

// TestNetDialerIsBounded pins the property every outbound path leans on: the shared
// dialer must impose a usable connect timeout. Zero is no timeout at all and hands the
// wait back to the OS - over two minutes on Linux. Negative is degenerate the other
// way: net.Dialer reads it as an already-expired deadline, so every dial fails at once.
// The behavioural tests here supply a timeout of their own, so none of them can catch
// either. What is pinned is the bound, not the value; 60s stays open to retuning.
func TestNetDialerIsBounded(t *testing.T) {
	t.Parallel()

	p := newTestProxy(t)

	if p.netDialer.Timeout <= 0 {
		t.Fatalf("netDialer.Timeout = %v, want > 0: an unbounded dial leaves the wait to the OS connect timeout", p.netDialer.Timeout)
	}
}

// TestBodyOutlastsResponseHeaderTimeout holds the proxy to the only bound it may impose
// on a response: the wait for headers. Once headers arrive the body may take as long as
// it takes, which is what large downloads, SSE and long-lived streams all depend on.
func TestBodyOutlastsResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	const (
		headerTimeout = 250 * time.Millisecond
		// Kept a multiple of headerTimeout rather than equal to it, so that retuning either
		// constant cannot land a write on top of the header deadline.
		chunkDelay = 2 * headerTimeout
		chunks     = 4
	)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rc := http.NewResponseController(w)
		// Flush the headers before the first sleep. The deadline being exercised runs only
		// until headers arrive, so without this the first body write would also be the
		// first header byte and the test would measure the wrong thing.
		w.WriteHeader(http.StatusOK)
		rc.Flush()

		// Total body time is chunks*chunkDelay, well past headerTimeout.
		for range chunks {
			time.Sleep(chunkDelay)
			io.WriteString(w, "x")
			rc.Flush()
		}
	}))
	defer target.Close()

	addr := startTestProxy(t, func(p *Proxy) {
		transportOf(t, p).ResponseHeaderTimeout = headerTimeout
	})
	client := proxyClient(t, addr)

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("get through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if want := strings.Repeat("x", chunks); string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

// TestStallBeforeHeadersReturns502 covers the bound itself: a server that accepts the
// connection and then never answers must not hold the client open indefinitely.
func TestStallBeforeHeadersReturns502(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	// Defer order is load-bearing. httptest.Server.Close waits for in-flight
	// handlers, and this handler only returns once released - the proxy hanging up
	// upstream does not wake it. Defers run last-in-first-out, so releasing must be
	// deferred after Close to run before it.
	defer target.Close()
	defer close(release)

	addr := startTestProxy(t, func(p *Proxy) {
		transportOf(t, p).ResponseHeaderTimeout = 100 * time.Millisecond
	})
	client := proxyClient(t, addr)

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("get through proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

// TestTunnelForwardsTraffic covers the CONNECT path end to end, so that changes to how
// the tunnel dials cannot quietly break the tunnel itself. No TLS is involved: httptest
// listens on 127.0.0.1, and proxyConnect sends bare IPs down the tunnel rather than
// MITM'ing them.
func TestTunnelForwardsTraffic(t *testing.T) {
	t.Parallel()

	const want = "through the tunnel"
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, want)
	}))
	defer target.Close()

	addr := startTestProxy(t, nil)

	conn, br, resp := connectThrough(t, addr, target.Listener.Addr().String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("build tunnelled request: %v", err)
	}
	if err := req.Write(conn); err != nil {
		t.Fatalf("write tunnelled request: %v", err)
	}

	tunnelled, err := http.ReadResponse(br, req)
	if err != nil {
		t.Fatalf("read tunnelled response: %v", err)
	}
	defer tunnelled.Body.Close()

	body, err := io.ReadAll(tunnelled.Body)
	if err != nil {
		t.Fatalf("read tunnelled body: %v", err)
	}
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

// localTestHost stands in for the local endpoint hostname. It sits under the
// reserved .test TLD, so a regression that sends it down a real tunnel can only
// fail resolution, never reach an actual host.
const localTestHost = "local.test"

// TestLocalEndpointServedOverCONNECT pins the endpoint's happy path: a CONNECT to the
// local host is MITM'd and answered by the local handler directly, with no upstream
// round trip involved.
func TestLocalEndpointServedOverCONNECT(t *testing.T) {
	t.Parallel()

	_, addr := startLocalEndpointProxy(t, noopFilter{}, nil)

	body := localEndpointGet(t, addr, localTestHost+":443")
	if body != localEndpointBody {
		t.Fatalf("body = %q, want %q", body, localEndpointBody)
	}
}

// TestLocalEndpointHostIsCaseInsensitive pins that a client sending the authority in a
// different case still reaches the handler rather than a tunnel.
func TestLocalEndpointHostIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	_, addr := startLocalEndpointProxy(t, noopFilter{}, nil)

	body := localEndpointGet(t, addr, "LOCAL.TEST:443")
	if body != localEndpointBody {
		t.Fatalf("body = %q, want %q", body, localEndpointBody)
	}
}

// TestLocalEndpointWinsOverTransparentHosts guards the check ordering in proxyConnect:
// a TLS failure elsewhere can land a host on transparentHosts, and for any other host
// that means tunnelling from then on. The local endpoint must keep being served - a
// tunnel would dial the hostname for real and break the endpoint until restart.
func TestLocalEndpointWinsOverTransparentHosts(t *testing.T) {
	t.Parallel()

	p, addr := startLocalEndpointProxy(t, noopFilter{}, nil)
	p.addTransparentHost(localTestHost)

	body := localEndpointGet(t, addr, localTestHost+":443")
	if body != localEndpointBody {
		t.Fatalf("body = %q, want %q", body, localEndpointBody)
	}
}

// TestLocalEndpointServedOverPlainHTTP covers the proxyHTTP path: nothing injects
// plain-http URLs, but the endpoint behaves the same on both schemes.
func TestLocalEndpointServedOverPlainHTTP(t *testing.T) {
	t.Parallel()

	_, addr := startLocalEndpointProxy(t, noopFilter{}, nil)

	if body := localEndpointPlainGet(t, addr); body != localEndpointBody {
		t.Fatalf("body = %q, want %q", body, localEndpointBody)
	}
}

// TestLocalEndpointTLSFailureDoesNotDisableMITM pins the transparentHosts exemption:
// for any other host a failed MITM handshake records the host and tunnels it from
// then on, which self-limits the list to one entry per host. The endpoint keeps
// being MITM'd instead, so without the exemption every failed handshake would
// append another copy of its name.
func TestLocalEndpointTLSFailureDoesNotDisableMITM(t *testing.T) {
	t.Parallel()

	p, addr := startLocalEndpointProxy(t, noopFilter{}, nil)

	conn, _, resp := connectThrough(t, addr, localTestHost+":443")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	// A verifying client is the production trigger: it rejects the self-signed
	// leaf with a bad-certificate alert, which the server sees as a TLS error.
	tlsConn := tls.Client(conn, &tls.Config{ServerName: localTestHost, MinVersion: tls.VersionTLS12})
	if err := tlsConn.Handshake(); err == nil {
		t.Fatal("handshake succeeded, want the client to reject the self-signed leaf")
	}
	// The client-side failure does not order with the server's bookkeeping;
	// proxyConnect closing the connection on return does, so drain until then.
	_, _ = io.Copy(io.Discard, conn)

	p.transparentHostsMu.RLock()
	got := append([]string(nil), p.transparentHosts...)
	p.transparentHostsMu.RUnlock()
	if len(got) != 0 {
		t.Fatalf("transparentHosts = %q, want empty: a failed local-endpoint handshake must not record the host", got)
	}

	if body := localEndpointGet(t, addr, localTestHost+":443"); body != localEndpointBody {
		t.Fatalf("body = %q, want %q", body, localEndpointBody)
	}
}

// TestLocalEndpointWinsOverShouldProxyExclusion guards the remaining check ordering
// in proxyConnect and proxyHTTP: a routing policy that excludes the requesting
// process tunnels every other host, and a tunnel would dial the endpoint's
// hostname for real. The consulted flag keeps the test honest - shouldProxy is
// only reached when the requesting process is identified, so if process lookup
// ever breaks under test, this fails loudly instead of passing vacuously.
func TestLocalEndpointWinsOverShouldProxyExclusion(t *testing.T) {
	t.Parallel()

	var consulted atomic.Bool
	excludeEverything := func(string) bool {
		consulted.Store(true)
		return false
	}
	_, addr := startLocalEndpointProxy(t, noopFilter{}, excludeEverything)

	if body := localEndpointGet(t, addr, localTestHost+":443"); body != localEndpointBody {
		t.Fatalf("CONNECT body = %q, want %q", body, localEndpointBody)
	}

	if body := localEndpointPlainGet(t, addr); body != localEndpointBody {
		t.Fatalf("plain-HTTP body = %q, want %q", body, localEndpointBody)
	}

	if !consulted.Load() {
		t.Fatal("shouldProxy was never consulted: process lookup did not identify the test process, so the exclusion ordering went unexercised")
	}
}

// TestLocalEndpointImmuneToFilter pins that no filter rule can block the endpoint:
// on the CONNECT path its requests never reach the filter (the local handler
// replaces the round-tripping one), and on the plain-HTTP path the endpoint check
// runs before the filter. The blocked.test probe proves the filter is live rather
// than accidentally disconnected.
func TestLocalEndpointImmuneToFilter(t *testing.T) {
	t.Parallel()

	_, addr := startLocalEndpointProxy(t, blockEverythingFilter{}, nil)

	if status, body := mitmGet(t, addr, "blocked.test:443", "blocked.test"); status != http.StatusForbidden || body != blockedBody {
		t.Fatalf("blocked.test: status = %d, body = %q, want %d, %q", status, body, http.StatusForbidden, blockedBody)
	}

	if body := localEndpointGet(t, addr, localTestHost+":443"); body != localEndpointBody {
		t.Fatalf("CONNECT body = %q, want %q", body, localEndpointBody)
	}

	if body := localEndpointPlainGet(t, addr); body != localEndpointBody {
		t.Fatalf("plain-HTTP body = %q, want %q", body, localEndpointBody)
	}
}

// backstopTimeout keeps a regression from hanging CI. It sits far above every delay
// these tests exercise, so it never fires on a healthy path.
const backstopTimeout = 10 * time.Second

// newTestProxy builds a Proxy without starting it.
func newTestProxy(t *testing.T) *Proxy {
	t.Helper()

	p, err := NewProxy(noopFilter{}, unusedCertGenerator{}, 0, nil, "", nil)
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	return p
}

// localEndpointBody is what the local endpoint handler installed by
// startLocalEndpointProxy answers every request with.
const localEndpointBody = "served locally"

// startLocalEndpointProxy starts a proxy configured to serve a handler on
// localTestHost and returns it along with its address.
func startLocalEndpointProxy(t *testing.T, f filter, shouldProxy ShouldProxyFunc) (*Proxy, string) {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, localEndpointBody)
	})
	p, err := NewProxy(f, selfSignedCertGenerator{}, 0, shouldProxy, localTestHost, handler)
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	port, err := p.Start()
	if err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	t.Cleanup(func() {
		if err := p.Stop(); err != nil {
			t.Errorf("stop proxy: %v", err)
		}
	})

	return p, fmt.Sprintf("127.0.0.1:%d", port)
}

// localEndpointGet CONNECTs to target through the proxy at proxyAddr, completes the
// MITM TLS handshake, and returns the body of a GET / served inside the tunnel.
func localEndpointGet(t *testing.T, proxyAddr, target string) string {
	t.Helper()

	_, body := mitmGet(t, proxyAddr, target, localTestHost)
	return body
}

// localEndpointPlainGet requests http://localTestHost/ through the proxy at
// proxyAddr - the proxyHTTP path, no tunnel - and returns the body.
func localEndpointPlainGet(t *testing.T, proxyAddr string) string {
	t.Helper()

	resp, err := proxyClient(t, proxyAddr).Get("http://" + localTestHost + "/")
	if err != nil {
		t.Fatalf("get through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// mitmGet CONNECTs to target through the proxy at proxyAddr, completes the MITM
// TLS handshake with serverName, and returns the status and body of a GET
// https://serverName/ served inside the tunnel.
func mitmGet(t *testing.T, proxyAddr, target, serverName string) (int, string) {
	t.Helper()

	conn, _, resp := connectThrough(t, proxyAddr, target)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true, // #nosec G402 -- the MITM certificate is self-signed on purpose; trust is not under test.
	})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+serverName+"/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if err := req.Write(tlsConn); err != nil {
		t.Fatalf("write request: %v", err)
	}

	tunnelled, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer tunnelled.Body.Close()

	body, err := io.ReadAll(tunnelled.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	return tunnelled.StatusCode, string(body)
}

// startTestProxy starts a proxy and returns its address. configure, if non-nil, may
// shorten timeouts before the proxy begins serving; it has to run there, because the
// transport and the dialer read their timeout fields without synchronisation and Start
// is what hands the proxy to serving goroutines.
func startTestProxy(t *testing.T, configure func(*Proxy)) string {
	t.Helper()

	p := newTestProxy(t)

	if configure != nil {
		configure(p)
	}

	port, err := p.Start()
	if err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	t.Cleanup(func() {
		if err := p.Stop(); err != nil {
			t.Errorf("stop proxy: %v", err)
		}
	})

	return fmt.Sprintf("127.0.0.1:%d", port)
}

// proxyClient returns a client that routes its requests through the proxy at addr.
func proxyClient(t *testing.T, addr string) *http.Client {
	t.Helper()

	proxyURL, err := url.Parse("http://" + addr)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}

	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   backstopTimeout,
	}
}

// connectThrough asks the proxy at proxyAddr to tunnel to target and returns the reply
// along with the connection and reader it arrived on, so a caller that got a 200 can
// keep speaking down the tunnel. Read what follows from the returned reader, not from
// the reply: a 200 to CONNECT carries no body, so everything after the status line has
// already been buffered here.
func connectThrough(t *testing.T, proxyAddr, target string) (net.Conn, *bufio.Reader, *http.Response) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", proxyAddr, backstopTimeout)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(backstopTimeout)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	req := &http.Request{
		Method: http.MethodConnect,
		// Opaque rather than Path: Request.Write only emits the authority form that
		// CONNECT requires when the URL carries no path.
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: make(http.Header),
	}
	if err := req.Write(conn); err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}

	return conn, br, resp
}

// transportOf returns the proxy's outbound transport, which is held behind an interface.
func transportOf(t *testing.T, p *Proxy) *http.Transport {
	t.Helper()

	transport, ok := p.requestTransport.(*http.Transport)
	if !ok {
		t.Fatalf("requestTransport is %T, want *http.Transport", p.requestTransport)
	}

	return transport
}

// noopFilter passes every request and response through untouched.
type noopFilter struct{}

func (noopFilter) HandleRequest(*http.Request, process.Info) (*http.Response, error) {
	return nil, nil
}

func (noopFilter) HandleResponse(*http.Request, *http.Response, process.Info) error {
	return nil
}

// blockedBody is what blockEverythingFilter answers every request with.
const blockedBody = "blocked by test filter"

// blockEverythingFilter blocks every request with a canned response, standing in
// for a filter list rule that happens to match Zen's own assets.
type blockEverythingFilter struct{}

func (blockEverythingFilter) HandleRequest(*http.Request, process.Info) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(blockedBody)),
	}, nil
}

func (blockEverythingFilter) HandleResponse(*http.Request, *http.Response, process.Info) error {
	return nil
}

// unusedCertGenerator satisfies NewProxy's non-nil check. Certificates are only
// generated for MITM'd CONNECT requests, so it is never called on the plain-HTTP
// path these tests exercise.
type unusedCertGenerator struct{}

func (unusedCertGenerator) GetCertificate(string) (*tls.Certificate, error) {
	return nil, errors.New("not implemented")
}

// selfSignedCertGenerator mints a throwaway self-signed leaf per host, standing in
// for the CA-backed generator on MITM paths.
type selfSignedCertGenerator struct{}

func (selfSignedCertGenerator) GetCertificate(host string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}, nil
}

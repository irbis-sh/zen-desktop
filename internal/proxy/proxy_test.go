package proxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/process"
)

// TestRequestClientHasNoTotalTimeout is the direct guard for the bug this package
// used to have: http.Client.Timeout covers the response body read, so setting it
// caps how long a transfer may take and truncates large downloads and long-lived
// streams mid-body. No behavioural test can stand in for this one, because any
// value small enough to exercise in a test would also have to be reintroduced to
// be observed.
func TestRequestClientHasNoTotalTimeout(t *testing.T) {
	t.Parallel()

	p := newTestProxy(t)

	if p.requestClient.Timeout != 0 {
		t.Fatalf("requestClient.Timeout = %v, want 0: a total timeout bounds the body read and truncates long transfers", p.requestClient.Timeout)
	}
}

// TestResponseHeaderTimeoutPreservesOldBudget pins the invariant that makes the
// removal of the total timeout safe. The header wait replaced a 60s total timeout,
// so as long as it stays at or above 60s, no request that used to succeed can begin
// to fail: headers already had to arrive inside the old budget.
func TestResponseHeaderTimeoutPreservesOldBudget(t *testing.T) {
	t.Parallel()

	p := newTestProxy(t)

	transport, ok := p.requestTransport.(*http.Transport)
	if !ok {
		t.Fatalf("requestTransport is %T, want *http.Transport", p.requestTransport)
	}

	const previousTotalTimeout = 60 * time.Second
	if transport.ResponseHeaderTimeout < previousTotalTimeout {
		t.Fatalf("ResponseHeaderTimeout = %v, want >= %v so that requests which succeeded under the old total timeout still succeed", transport.ResponseHeaderTimeout, previousTotalTimeout)
	}
}

// TestBodyOutlastsResponseHeaderTimeout proves the bound that replaced the total
// timeout did not recreate it: a response body may take arbitrarily longer than the
// header wait. Without this, the fix could be silently undone by moving the ceiling
// from one field to another.
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

	client := startTestProxy(t, headerTimeout)

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

// TestStallBeforeHeadersReturns502 proves the replacement bound actually fires.
// Without it, TestBodyOutlastsResponseHeaderTimeout would also pass if the header
// timeout were never applied at all.
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

	client := startTestProxy(t, 100*time.Millisecond)

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("get through proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

// newTestProxy builds a Proxy without starting it.
func newTestProxy(t *testing.T) *Proxy {
	t.Helper()

	p, err := NewProxy(noopFilter{}, unusedCertGenerator{}, 0, nil)
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	return p
}

// startTestProxy starts a proxy with a shortened response header timeout and returns
// a client routed through it.
func startTestProxy(t *testing.T, headerTimeout time.Duration) *http.Client {
	t.Helper()

	p := newTestProxy(t)

	// This must happen before Start. http.Transport reads its timeout fields without
	// synchronisation, and Start is what hands the proxy to serving goroutines, so
	// mutating the field afterwards is a data race.
	transport, ok := p.requestTransport.(*http.Transport)
	if !ok {
		t.Fatalf("requestTransport is %T, want *http.Transport", p.requestTransport)
	}
	transport.ResponseHeaderTimeout = headerTimeout

	port, err := p.Start()
	if err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	t.Cleanup(func() {
		if err := p.Stop(); err != nil {
			t.Errorf("stop proxy: %v", err)
		}
	})

	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}

	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		// A backstop so a regression fails the test rather than hanging CI. It sits far
		// above every delay used here and never fires on the paths being exercised.
		Timeout: 10 * time.Second,
	}
}

// noopFilter passes every request and response through untouched.
type noopFilter struct{}

func (noopFilter) HandleRequest(*http.Request, process.Info) (*http.Response, error) {
	return nil, nil
}

func (noopFilter) HandleResponse(*http.Request, *http.Response, process.Info) error {
	return nil
}

// unusedCertGenerator satisfies NewProxy's non-nil check. Certificates are only
// generated for MITM'd CONNECT requests, so it is never called on the plain-HTTP
// path these tests exercise.
type unusedCertGenerator struct{}

func (unusedCertGenerator) GetCertificate(string) (*tls.Certificate, error) {
	return nil, errors.New("not implemented")
}

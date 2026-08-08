package localcdn_test

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
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/config"
	"github.com/irbis-sh/zen-desktop/internal/localcdn"
	"github.com/irbis-sh/zen-desktop/internal/process"
	"github.com/irbis-sh/zen-desktop/internal/proxy"
)

// localOnlyFilter passes requests through to the local resource engine and
// does not perform any other filtering.
type localOnlyFilter struct {
	engine *localcdn.Engine
}

func (f *localOnlyFilter) HandleRequest(req *http.Request, _ process.Info) (*http.Response, error) {
	resp, _, err := f.engine.HandleRequest(req)
	return resp, err
}

func (f *localOnlyFilter) HandleResponse(_ *http.Request, _ *http.Response, _ process.Info) error {
	return nil
}

type integrationCertGenerator struct{}

func (integrationCertGenerator) GetCertificate(string) (*tls.Certificate, error) {
	return nil, errors.New("unused in tests")
}

// staticCertGenerator returns the same certificate for every host. It is used
// to exercise the CONNECT/MITM path with real TLS in tests.
type staticCertGenerator struct {
	cert *tls.Certificate
}

func (g *staticCertGenerator) GetCertificate(string) (*tls.Certificate, error) {
	return g.cert, nil
}

func startIntegrationProxy(t *testing.T, engine *localcdn.Engine) int {
	t.Helper()
	p, err := proxy.NewProxy(&localOnlyFilter{engine: engine}, integrationCertGenerator{}, 0, nil)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	port, err := p.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = p.Stop()
	})
	return port
}

func startIntegrationProxyWithCert(t *testing.T, engine *localcdn.Engine, certGenerator proxyCertGenerator) int {
	t.Helper()
	p, err := proxy.NewProxy(&localOnlyFilter{engine: engine}, certGenerator, 0, nil)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	port, err := p.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = p.Stop()
	})
	return port
}

// proxyCertGenerator matches the proxy package's cert generator contract.
type proxyCertGenerator interface {
	GetCertificate(host string) (*tls.Certificate, error)
}

func newTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Zen LocalCDN Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	return cert, key
}

func newTestLeafCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, host string) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func integrationProxyGet(t *testing.T, port int, rawURL string) *http.Response {
	t.Helper()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", rawURL, u.Host)

	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp
}

// integrationProxyConnectMITM opens a CONNECT tunnel to the proxy, performs a
// TLS handshake with the given CA, and returns the established TLS connection.
func integrationProxyConnectMITM(t *testing.T, port int, host string, caPool *x509.CertPool) *tls.Conn {
	t.Helper()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	fmt.Fprintf(conn, "CONNECT %s:443 HTTP/1.1\r\nHost: %s:443\r\n\r\n", host, host)
	reader := bufio.NewReader(conn)
	connectResp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if connectResp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want 200", connectResp.StatusCode)
	}

	tlsConn := tls.Client(conn, &tls.Config{
		RootCAs:    caPool,
		ServerName: host,
		NextProtos: []string{"http/1.1"},
	})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}
	return tlsConn
}

func TestProxyServesLocalResource(t *testing.T) {
	t.Parallel()

	engine, err := localcdn.NewEngine(localcdn.Options{
		Settings: config.LocalResources{Enabled: true},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	port := startIntegrationProxy(t, engine)

	resp := integrationProxyGet(t, port, "http://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Fatalf("content type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", cc)
	}
	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "*" {
		t.Fatalf("access-control-allow-origin = %q", acao)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "jQuery v3.7.1") {
		t.Fatal("response body does not look like jQuery")
	}
}

func TestProxyBlocksMissingResource(t *testing.T) {
	t.Parallel()

	engine, err := localcdn.NewEngine(localcdn.Options{
		Settings: config.LocalResources{Enabled: true, BlockMissing: true},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	port := startIntegrationProxy(t, engine)

	resp := integrationProxyGet(t, port, "http://cdnjs.cloudflare.com/ajax/libs/not-a-real-library/9.9.9/not-a-real-library.js")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestProxyServesBootstrapCSS(t *testing.T) {
	t.Parallel()

	engine, err := localcdn.NewEngine(localcdn.Options{
		Settings: config.LocalResources{Enabled: true},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	port := startIntegrationProxy(t, engine)

	resp := integrationProxyGet(t, port, "http://cdn.jsdelivr.net/npm/bootstrap@5.3.8/dist/css/bootstrap.min.css")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("content type = %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "--bs-") {
		t.Fatal("response body does not look like Bootstrap CSS")
	}
}

func TestProxyServesLocalResourceOverMITM(t *testing.T) {
	t.Parallel()

	const cdnHost = "cdnjs.cloudflare.com"
	ca, caKey := newTestCA(t)
	leaf := newTestLeafCert(t, ca, caKey, cdnHost)
	caPool := x509.NewCertPool()
	caPool.AddCert(ca)

	engine, err := localcdn.NewEngine(localcdn.Options{
		Settings: config.LocalResources{Enabled: true},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	port := startIntegrationProxyWithCert(t, engine, &staticCertGenerator{cert: leaf})

	tlsConn := integrationProxyConnectMITM(t, port, cdnHost, caPool)
	defer tlsConn.Close()

	fmt.Fprintf(tlsConn, "GET /ajax/libs/jquery/3.7.1/jquery.min.js HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", cdnHost)
	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Fatalf("content type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", cc)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "jQuery v3.7.1") {
		t.Fatal("response body does not look like jQuery")
	}
}

func TestProxyBlocksMissingResourceOverMITM(t *testing.T) {
	t.Parallel()

	const cdnHost = "cdnjs.cloudflare.com"
	ca, caKey := newTestCA(t)
	leaf := newTestLeafCert(t, ca, caKey, cdnHost)
	caPool := x509.NewCertPool()
	caPool.AddCert(ca)

	engine, err := localcdn.NewEngine(localcdn.Options{
		Settings: config.LocalResources{Enabled: true, BlockMissing: true},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	port := startIntegrationProxyWithCert(t, engine, &staticCertGenerator{cert: leaf})

	tlsConn := integrationProxyConnectMITM(t, port, cdnHost, caPool)
	defer tlsConn.Close()

	fmt.Fprintf(tlsConn, "GET /ajax/libs/not-a-real-library/9.9.9/not-a-real-library.js HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", cdnHost)
	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

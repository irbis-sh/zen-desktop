package certgen

import (
	"bytes"
	"crypto"
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/certstore"
)

const testOrgName = "Zen Test"

func TestRootIssuerSignsLeavesDirectly(t *testing.T) {
	t.Parallel()

	ca, _ := newTestCA(t, false)
	cg, err := NewCertGenerator(fakeStore{ca}, testOrgName)
	if err != nil {
		t.Fatalf("NewCertGenerator: %v", err)
	}

	cert, err := cg.GetCertificate("example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if len(cert.Certificate) != 1 {
		t.Fatalf("served chain has %d certificates, want 1", len(cert.Certificate))
	}
	verifyChain(t, cert, ca.Cert, "example.com", time.Now())
}

func TestIntermediateIssuerServesChain(t *testing.T) {
	t.Parallel()

	ca, _ := newTestCA(t, true)
	cg, err := NewCertGenerator(fakeStore{ca}, testOrgName)
	if err != nil {
		t.Fatalf("NewCertGenerator: %v", err)
	}

	cert, err := cg.GetCertificate("example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if len(cert.Certificate) != 2 {
		t.Fatalf("served chain has %d certificates, want 2", len(cert.Certificate))
	}
	intermediate := parseCert(t, cert.Certificate[1])
	if !intermediate.IsCA || intermediate.Subject.CommonName != intermediateCommonName {
		t.Errorf("second certificate is not the intermediate: %v", intermediate.Subject)
	}
	verifyChain(t, cert, ca.Cert, "example.com", time.Now())
}

func TestIntermediateIssuerClampsLeafToIntermediate(t *testing.T) {
	t.Parallel()

	ca, _ := newTestCA(t, true)
	clock := newFakeClock()
	cg := newCertGenerator(ca, testOrgName, clock.Now)

	first, err := cg.GetCertificate("a.example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	intermediate := parseCert(t, first.Certificate[1])

	// Late in the intermediate's life, but before it is due for renewal.
	clock.Advance(intermediateTTL - time.Hour)
	late, err := cg.GetCertificate("b.example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if !late.Leaf.NotAfter.Equal(intermediate.NotAfter) {
		t.Errorf("leaf NotAfter = %v, want the intermediate's %v", late.Leaf.NotAfter, intermediate.NotAfter)
	}
	verifyChain(t, late, ca.Cert, "b.example.com", clock.Now())
}

func TestIntermediateIssuerRotatesAfterExpiry(t *testing.T) {
	t.Parallel()

	ca, signer := newTestCA(t, true)
	clock := newFakeClock()
	cg := newCertGenerator(ca, testOrgName, clock.Now)

	first, err := cg.GetCertificate("a.example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got := signer.calls.Load(); got != 1 {
		t.Fatalf("root signatures after the first leaf = %d, want 1", got)
	}
	if _, err := cg.GetCertificate("b.example.com"); err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got := signer.calls.Load(); got != 1 {
		t.Fatalf("root signatures after the second leaf = %d, want 1", got)
	}

	clock.Advance(intermediateTTL + time.Second)
	rotated, err := cg.GetCertificate("c.example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got := signer.calls.Load(); got != 2 {
		t.Errorf("root signatures after expiry = %d, want 2", got)
	}
	oldIntermediate := parseCert(t, first.Certificate[1])
	newIntermediate := parseCert(t, rotated.Certificate[1])
	if oldIntermediate.SerialNumber.Cmp(newIntermediate.SerialNumber) == 0 {
		t.Error("leaf after expiry should chain to a new intermediate")
	}
	verifyChain(t, rotated, ca.Cert, "c.example.com", clock.Now())
}

func TestIntermediateIssuerSurvivesClockGoingBack(t *testing.T) {
	t.Parallel()

	ca, signer := newTestCA(t, true)
	clock := newFakeClock()
	cg := newCertGenerator(ca, testOrgName, clock.Now)
	if _, err := cg.GetCertificate("a.example.com"); err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}

	// A small correction is covered by backdating.
	clock.Advance(-backdate / 2)
	small, err := cg.GetCertificate("b.example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got := signer.calls.Load(); got != 1 {
		t.Errorf("root signatures after a small correction = %d, want 1", got)
	}
	verifyChain(t, small, ca.Cert, "b.example.com", clock.Now())

	// A larger one leaves the intermediate not valid yet, so it is replaced.
	clock.Advance(-3 * time.Hour)
	large, err := cg.GetCertificate("c.example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got := signer.calls.Load(); got != 2 {
		t.Errorf("root signatures after a large correction = %d, want 2", got)
	}
	if intermediate := parseCert(t, large.Certificate[1]); clock.Now().Before(intermediate.NotBefore) {
		t.Errorf("intermediate NotBefore %v is after now %v", intermediate.NotBefore, clock.Now())
	}
}

func TestIntermediateIssuerKeepsCurrentWhenRotationFails(t *testing.T) {
	t.Parallel()

	ca, signer := newTestCA(t, true)
	clock := newFakeClock()
	cg := newCertGenerator(ca, testOrgName, clock.Now)

	first, err := cg.GetCertificate("a.example.com")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	signer.fail.Store(true)

	// Inside the renew margin: the rotation fails, and the still-valid intermediate is used.
	clock.Advance(intermediateTTL - 2*time.Minute)
	kept, err := cg.GetCertificate("b.example.com")
	if err != nil {
		t.Fatalf("GetCertificate after a failed rotation: %v", err)
	}
	if !bytes.Equal(kept.Certificate[1], first.Certificate[1]) {
		t.Error("leaf after a failed rotation should chain to the current intermediate")
	}
	verifyChain(t, kept, ca.Cert, "b.example.com", clock.Now())
	if _, err := cg.GetCertificate("c.example.com"); err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got := signer.calls.Load(); got != 2 {
		t.Errorf("root signatures before the retry interval = %d, want 2", got)
	}

	clock.Advance(intermediateRetryInterval)
	if _, err := cg.GetCertificate("d.example.com"); err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got := signer.calls.Load(); got != 3 {
		t.Errorf("root signatures after the retry interval = %d, want 3", got)
	}

	clock.Advance(2 * time.Minute)
	if _, err := cg.GetCertificate("e.example.com"); err == nil {
		t.Error("GetCertificate succeeded after the intermediate expired and rotation failed")
	}
}

func TestIntermediateIssuerSignsOnceUnderConcurrency(t *testing.T) {
	t.Parallel()

	ca, signer := newTestCA(t, true)
	cg := newCertGenerator(ca, testOrgName, time.Now)

	const requests = 100
	var wg sync.WaitGroup
	errs := make(chan error, requests)
	for i := range requests {
		wg.Go(func() {
			if _, err := cg.GetCertificate(fmt.Sprintf("host%d.example.com", i)); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("GetCertificate: %v", err)
	}

	if got := signer.calls.Load(); got != 1 {
		t.Errorf("root signatures = %d, want 1", got)
	}
}

type fakeStore struct {
	ca certstore.CA
}

func (s fakeStore) GetCA() (certstore.CA, error) { return s.ca, nil }

// countingSigner counts the signatures made with the root key, standing in for the hardware.
// Setting fail makes it fail like a broken hardware key.
type countingSigner struct {
	key   *ecdsa.PrivateKey
	calls atomic.Int64
	fail  atomic.Bool
}

func (s *countingSigner) Public() crypto.PublicKey { return s.key.Public() }

func (s *countingSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.calls.Add(1)
	if s.fail.Load() {
		return nil, errors.New("signer failed")
	}
	return s.key.Sign(rand, digest, opts)
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Now()} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newTestCA returns an in-memory P-256 root shaped like the one certstore creates. A hardware-
// backed root allows one intermediate below it.
func newTestCA(t *testing.T, hardwareBacked bool) (certstore.CA, *countingSigner) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate root key: %v", err)
	}
	signer := &countingSigner{key: key}

	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{testOrgName}, CommonName: "Zen Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	if hardwareBacked {
		tpl.MaxPathLen = 1
	} else {
		tpl.MaxPathLenZero = true
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create root: %v", err)
	}

	return certstore.CA{Cert: parseCert(t, der), Key: signer, HardwareBacked: hardwareBacked}, signer
}

func parseCert(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

// verifyChain verifies a served certificate against a pool holding only the root, with the rest
// of the served chain as intermediates.
func verifyChain(t *testing.T, cert *tls.Certificate, root *x509.Certificate, host string, at time.Time) {
	t.Helper()

	roots := x509.NewCertPool()
	roots.AddCert(root)
	intermediates := x509.NewCertPool()
	for _, der := range cert.Certificate[1:] {
		intermediates.AddCert(parseCert(t, der))
	}
	_, err := parseCert(t, cert.Certificate[0]).Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		DNSName:       host,
		CurrentTime:   at,
	})
	if err != nil {
		t.Errorf("verify chain: %v", err)
	}
}

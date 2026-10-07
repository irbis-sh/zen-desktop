//go:build darwin || windows

package hwkey

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests run against the platform key store: in development builds that is the login
// keychain on macOS and the software key storage provider on Windows. Each test uses its own
// key name, so they cannot touch Zen's real key.

func TestAvailable(t *testing.T) {
	if err := Available(); err != nil {
		t.Fatalf("Available: %v", err)
	}
}

func TestLifecycle(t *testing.T) {
	name := testKeyName(t)

	s, err := Create(name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pub, ok := s.Public().(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		t.Fatalf("Public() = %T, want a P-256 *ecdsa.PublicKey", s.Public())
	}
	assertSigns(t, s, pub)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	opened, err := Open(name)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !pub.Equal(opened.Public()) {
		t.Error("opened key has a different public key")
	}
	assertSigns(t, opened, pub)
	if err := opened.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Open(name); !errors.Is(err, ErrNotFound) {
		t.Errorf("Open after Delete: got %v, want ErrNotFound", err)
	}
	if err := Delete(name); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete after Delete: got %v, want ErrNotFound", err)
	}
}

func TestCreateReplacesExistingKey(t *testing.T) {
	name := testKeyName(t)

	first, err := Create(name)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	first.Close()
	second, err := Create(name)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	defer second.Close()

	opened, err := Open(name)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer opened.Close()
	if !second.Public().(*ecdsa.PublicKey).Equal(opened.Public()) {
		t.Error("Open should return the key from the latest Create")
	}
}

// TestCloseDuringSign closes a handle while other goroutines sign with it, as happens when the
// proxy restarts during a handshake. Run with -race. Every Sign must either produce a valid
// signature or report the handle closed.
func TestCloseDuringSign(t *testing.T) {
	s, err := Create(testKeyName(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pub := s.Public().(*ecdsa.PublicKey)
	digest := sha256.Sum256([]byte("zen"))

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for {
				sig, err := s.Sign(rand.Reader, digest[:], crypto.SHA256)
				if err != nil {
					if !strings.Contains(err.Error(), "closed") {
						t.Errorf("Sign: %v, want a closed handle error", err)
					}
					return
				}
				if !ecdsa.VerifyASN1(pub, digest[:], sig) {
					t.Error("signature does not verify")
					return
				}
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	s.Close()
	wg.Wait()
}

// TestCertificateChain uses the key the way Zen does: it self-signs a root CA with it, issues a
// leaf, and verifies the chain. A malformed signature surfaces as a verification error.
func TestCertificateChain(t *testing.T) {
	s, err := Create(testKeyName(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer s.Close()

	rootTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "hwkey test root"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTpl, rootTpl, s.Public(), s)
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatalf("parse root: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"example.net"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, root, &leafKey.PublicKey, s)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(root)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "example.net"}); err != nil {
		t.Fatalf("verify leaf: %v", err)
	}
}

// testKeyName returns a key name unique to this test run and deletes the key when the test ends.
func testKeyName(t *testing.T) string {
	t.Helper()
	name := "net.zenprivacy.zen.test." + t.Name() + "." + rand.Text()
	t.Cleanup(func() { _ = Delete(name) })
	return name
}

func assertSigns(t *testing.T, s crypto.Signer, pub *ecdsa.PublicKey) {
	t.Helper()
	digest := sha256.Sum256([]byte("zen"))
	sig, err := s.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		t.Error("signature does not verify")
	}
}

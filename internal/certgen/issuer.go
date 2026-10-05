package certgen

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"
)

const (
	// intermediateTTL is how long an intermediate is valid. Leaves are clamped to their
	// intermediate's expiry, so any value is correct; this one sets how often the hardware signs.
	intermediateTTL = 24 * time.Hour
	// intermediateRenewMargin is how close to expiry an intermediate gets replaced. It matches the
	// leaf cache buffer.
	intermediateRenewMargin = 5 * time.Minute
	// intermediateRetryInterval spaces out retries of a failed rotation while the current
	// intermediate is still valid, so a failing hardware key isn't asked on every handshake.
	intermediateRetryInterval = time.Minute
	// intermediateCommonName is the common name for intermediate certificates.
	intermediateCommonName = "Zen Personal CA Intermediate"
)

// issuer supplies the CA that signs leaves.
type issuer interface {
	// current returns the CA to sign the next leaf with and the DER certificates to serve after
	// the leaf.
	current() (cert *x509.Certificate, key crypto.Signer, chain [][]byte, err error)
}

// rootIssuer signs leaves with the root directly. It is used when the root's key is on disk.
type rootIssuer struct {
	cert *x509.Certificate
	key  crypto.Signer
}

func (r *rootIssuer) current() (*x509.Certificate, crypto.Signer, [][]byte, error) {
	return r.cert, r.key, nil, nil
}

// intermediateIssuer signs leaves with a short-lived intermediate whose key only lives in memory.
// It is used when the root's key is in hardware: the hardware then signs one intermediate a day
// rather than one leaf per host.
//
// Rotation is lazy and happens under mu, so concurrent requests at expiry produce exactly one
// hardware signature.
type intermediateIssuer struct {
	root    *x509.Certificate
	rootKey crypto.Signer
	orgName string
	now     func() time.Time

	mu   sync.Mutex
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	// retryAt is when a rotation that failed before expiry may be tried again.
	retryAt time.Time
}

func (i *intermediateIssuer) current() (*x509.Certificate, crypto.Signer, [][]byte, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	now := i.now()
	switch {
	// An intermediate that is not valid yet means the clock went back further than backdate. All
	// leaves chain to it, so it is replaced, not kept until the clock catches up.
	case i.cert == nil || !now.Before(i.cert.NotAfter) || now.Before(i.cert.NotBefore):
		if err := i.rotate(now); err != nil {
			return nil, nil, nil, err
		}
	case now.Add(intermediateRenewMargin).After(i.cert.NotAfter) && !now.Before(i.retryAt):
		if err := i.rotate(now); err != nil {
			// The current intermediate still works, so a failed signature need not stop the
			// proxy before it expires. Leaves issued meanwhile are clamped to its expiry.
			log.Printf("rotate intermediate CA, keeping the current one until it expires: %v", err)
			i.retryAt = now.Add(intermediateRetryInterval)
		}
	}
	return i.cert, i.key, [][]byte{i.cert.Raw}, nil
}

// rotate mints a new intermediate. The caller must hold mu.
func (i *intermediateIssuer) rotate(now time.Time) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate intermediate key: %w", err)
	}
	serialNumber, err := randomSerialNumber()
	if err != nil {
		return err
	}

	// SubjectKeyId is left empty: CreateCertificate derives it from the public key for CA
	// certificates.
	tpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{i.orgName},
			CommonName:   intermediateCommonName,
		},
		NotBefore: now.Add(-backdate),
		NotAfter:  now.Add(intermediateTTL),

		KeyUsage: x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, i.root, &key.PublicKey, i.rootKey)
	if err != nil {
		return fmt.Errorf("create intermediate certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return fmt.Errorf("parse intermediate certificate: %w", err)
	}

	i.cert, i.key = cert, key
	return nil
}

func randomSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	return serialNumber, nil
}

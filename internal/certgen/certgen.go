package certgen

import (
	"errors"
	"fmt"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/certstore"
)

const (
	// cacheMaxSize is the maximum number of certificates the cache will store.
	//
	// Considering that a single tls.Certificate is about 1.7KB, this means that the cache
	// can store 5800 certificates in about 10MB of memory.
	cacheMaxSize = 5800
	// cacheCleanupInterval is the interval at which the cache is cleaned up.
	cacheCleanupInterval = 5 * time.Minute
)

// certStore is an interface for getting the root CA.
type certStore interface {
	GetCA() (certstore.CA, error)
}

// CertGenerator allows for generating certificates for a given host.
type CertGenerator struct {
	cache   *certLRUCache
	issuer  issuer
	orgName string
	now     func() time.Time
}

// NewCertGenerator creates a generator for the store's current CA. The store must be
// initialised. A new generator is created for every proxy start, so a hardware root gets a fresh
// intermediate each time.
func NewCertGenerator(certStore certStore, orgName string) (*CertGenerator, error) {
	if certStore == nil {
		return nil, errors.New("certStore is nil")
	}
	if orgName == "" {
		return nil, errors.New("orgName is empty")
	}

	ca, err := certStore.GetCA()
	if err != nil {
		return nil, fmt.Errorf("get CA: %w", err)
	}

	return newCertGenerator(ca, orgName, time.Now), nil
}

func newCertGenerator(ca certstore.CA, orgName string, now func() time.Time) *CertGenerator {
	var iss issuer
	if ca.HardwareBacked {
		iss = &intermediateIssuer{root: ca.Cert, rootKey: ca.Key, orgName: orgName, now: now}
	} else {
		iss = &rootIssuer{cert: ca.Cert, key: ca.Key}
	}

	return &CertGenerator{
		cache:   newCertLRUCache(cacheMaxSize, cacheCleanupInterval),
		issuer:  iss,
		orgName: orgName,
		now:     now,
	}
}

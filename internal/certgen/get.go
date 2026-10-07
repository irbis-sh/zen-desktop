package certgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"time"
)

const (
	// certTTL is the time-to-live for certificates.
	certTTL = 24 * time.Hour
	// backdate moves NotBefore into the past on leaves and intermediates, so a certificate made
	// just before the system clock is corrected backwards is still valid.
	backdate = time.Hour
)

// GetCertificate returns a certificate for the given host, signed by the root CA or, with a
// hardware root, by the current intermediate.
func (cg *CertGenerator) GetCertificate(host string) (*tls.Certificate, error) {
	if cert := cg.cache.Get(host); cert != nil {
		return cert, nil
	}

	issuerCert, issuerKey, chain, err := cg.issuer.current()
	if err != nil {
		return nil, fmt.Errorf("get issuer: %v", err)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private key: %v", err)
	}

	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, err
	}

	now := cg.now()
	notAfter := now.Add(certTTL)
	// A leaf must never outlive its issuer. The cache expiry follows notAfter, so this also
	// keeps cached leaves from outliving a rotated intermediate.
	if issuerCert.NotAfter.Before(notAfter) {
		notAfter = issuerCert.NotAfter
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{cg.orgName},
		},
		NotBefore: now.Add(-backdate),
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, issuerCert, &privateKey.PublicKey, issuerKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %v", err)
	}

	cert := &tls.Certificate{
		Certificate: append([][]byte{derBytes}, chain...),
		PrivateKey:  privateKey,
		Leaf:        leaf,
	}

	cg.cache.Put(host, notAfter.Add(-5*time.Minute), cert) // 5 minute buffer in case a TLS handshake takes a while, the system clock is off, etc.

	return cert, nil
}

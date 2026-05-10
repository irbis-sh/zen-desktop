package certgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// certTTL is the time-to-live for certificates.
const certTTL = 24 * time.Hour

// GetCertificate returns a self-signed certificate for the given host.
func (cg *CertGenerator) GetCertificate(host string) (*tls.Certificate, error) {
	rootCert, rootKey, err := cg.store.GetCertificate()
	if err != nil {
		return nil, fmt.Errorf("get root certificate: %v", err)
	}

	if cert := cg.cache.Get(host); cert != nil {
		return cert, nil
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %v", err)
	}

	notAfter := time.Now().Add(certTTL)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{cg.orgName},
		},
		NotBefore: time.Now(),
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, rootCert, &privateKey.PublicKey, rootKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %v", err)
	}
	pemCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if pemCert == nil {
		return nil, fmt.Errorf("encode certificate to PEM")
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	if pemCert == nil {
		return nil, fmt.Errorf("encode key to PEM")
	}

	cert, err := tls.X509KeyPair(pemCert, pemKey)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %v", err)
	}

	cg.cache.Put(host, notAfter.Add(-5*time.Minute), &cert) // 5 minute buffer in case a TLS handshake takes a while, the system clock is off, etc.

	return &cert, nil
}

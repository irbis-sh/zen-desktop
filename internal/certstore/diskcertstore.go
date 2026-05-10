// Package certstore implements a certificate store.
package certstore

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- SHA-1 is used for certificate fingerprinting, not for hashing passwords or data.
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/hectane/go-acl"
)

const (
	// certFilename is the name of the file containing the root CA certificate.
	certFilename = "rootCA.pem"
	// keyFilename is the name of the file containing the root CA key.
	keyFilename = "rootCA-key.pem"
	// certCommonName is the common name for the root CA certificate.
	certCommonName = "Zen Personal CA"
)

type CAStatusManager interface {
	GetCAInstalled() bool
	SetCAInstalled(value bool)
}

// DiskCertStore is a disk-based certificate store.
// It manages the creation, loading, and installation of the root CA.
type DiskCertStore struct {
	mu              sync.RWMutex
	caStatusManager CAStatusManager
	folderPath      string
	certData        []byte
	keyData         []byte
	certPath        string
	cert            *x509.Certificate
	keyPath         string
	key             crypto.PrivateKey
	orgName         string
}

func NewDiskCertStore(caStatusManager CAStatusManager, dataDir string, orgName string) (*DiskCertStore, error) {
	if caStatusManager == nil {
		return nil, errors.New("caStatusManager is nil")
	}
	if dataDir == "" {
		return nil, errors.New("dataDir is nil")
	}
	if orgName == "" {
		return nil, errors.New("orgName is nil")
	}

	cs := &DiskCertStore{}
	cs.caStatusManager = caStatusManager
	cs.folderPath = filepath.Join(dataDir, caFolderName)
	cs.certPath = filepath.Join(cs.folderPath, certFilename)
	cs.keyPath = filepath.Join(cs.folderPath, keyFilename)
	cs.orgName = orgName

	return cs, nil
}

func (cs *DiskCertStore) GetCertificate() (*x509.Certificate, crypto.PrivateKey, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.cert == nil || cs.key == nil {
		return nil, nil, errors.New("CA not initialized")
	}

	return cs.cert, cs.key, nil
}

func (cs *DiskCertStore) Init() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.caStatusManager.GetCAInstalled() {
		if err := cs.loadCA(); err != nil {
			return fmt.Errorf("CA load: %w", err)
		}
		return nil
	}

	if err := os.RemoveAll(cs.folderPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing CA folder: %v", err)
	}
	if err := os.MkdirAll(cs.folderPath, 0755); err != nil {
		return fmt.Errorf("create certs folder: %v", err)
	}
	if err := cs.newCA(); err != nil {
		return fmt.Errorf("create new CA: %v", err)
	}
	if err := cs.loadCA(); err != nil {
		return fmt.Errorf("CA load: %v", err)
	}
	if err := cs.installCATrust(); err != nil {
		return fmt.Errorf("install CA from system trust store: %v", err)
	}
	if err := cs.installNSS(); err != nil {
		log.Printf("install CA from NSS database: %v", err)
	}
	cs.caStatusManager.SetCAInstalled(true)

	return nil
}

func (cs *DiskCertStore) UninstallCA() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.caStatusManager.GetCAInstalled() {
		return errors.New("CA not installed")
	}

	if cs.cert == nil || cs.key == nil {
		if err := cs.loadCA(); err != nil {
			return fmt.Errorf("CA load: %v", err)
		}
	}

	if err := cs.uninstallCATrust(); err != nil {
		return fmt.Errorf("uninstall CA from system trust store: %w", err)
	}
	if err := cs.uninstallNSS(); err != nil {
		log.Printf("uninstall CA from NSS database: %v", err)
	}
	if err := os.RemoveAll(cs.folderPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove CA folder: %w", err)
	}

	cs.caStatusManager.SetCAInstalled(false)

	return nil
}

// newCA creates a new CA certificate/key pair and saves it to disk.
func (cs *DiskCertStore) newCA() error {
	priv, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return fmt.Errorf("generate key: %v", err)
	}
	pub := priv.Public()

	spkiASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("marshal public key: %v", err)
	}

	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	_, err = asn1.Unmarshal(spkiASN1, &spki)
	if err != nil {
		return fmt.Errorf("unmarshal public key: %v", err)
	}

	skid := sha1.Sum(spki.SubjectPublicKey.Bytes) // #nosec G401

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("generate serial number: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{cs.orgName},
			CommonName:   certCommonName,
		},
		SubjectKeyId: skid[:],

		NotAfter:  time.Now().AddDate(32, 0, 0),
		NotBefore: time.Now(),

		KeyUsage: x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	cert, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		return fmt.Errorf("create certificate: %v", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal private key: %v", err)
	}
	err = os.WriteFile(cs.keyPath, pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0600)
	if err != nil {
		return fmt.Errorf("write private key at %s: %v", cs.keyPath, err)
	}
	if runtime.GOOS == "windows" {
		// 0600 to allow the current user to read/write/delete the file
		if err := acl.Chmod(cs.keyPath, 0600); err != nil {
			return fmt.Errorf("chmod private key at %s: %v", cs.keyPath, err)
		}
	}

	err = os.WriteFile(cs.certPath, pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0644)
	if err != nil {
		return fmt.Errorf("write certificate at %s: %v", cs.certPath, err)
	}
	if runtime.GOOS == "windows" {
		if err := acl.Chmod(cs.certPath, 0644); err != nil {
			return fmt.Errorf("chmod certificate at %s: %v", cs.certPath, err)
		}
	}

	return nil
}

// loadCA loads the existing CA certificate and key into memory.
func (cs *DiskCertStore) loadCA() error {
	if _, err := os.Stat(cs.certPath); os.IsNotExist(err) {
		return fmt.Errorf("CA cert does not exist at %s", cs.certPath)
	}
	if _, err := os.Stat(cs.keyPath); os.IsNotExist(err) {
		return fmt.Errorf("CA key does not exist at %s", cs.keyPath)
	}

	var err error
	cs.certData, err = os.ReadFile(cs.certPath)
	if err != nil {
		return fmt.Errorf("read CA cert: %v", err)
	}
	certDERBlock, _ := pem.Decode(cs.certData)
	if certDERBlock == nil || certDERBlock.Type != "CERTIFICATE" {
		return errors.New("CA cert type mismatch")
	}
	cs.cert, err = x509.ParseCertificate(certDERBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse CA cert: %v", err)
	}

	cs.keyData, err = os.ReadFile(cs.keyPath)
	if err != nil {
		return fmt.Errorf("read CA key: %v", err)
	}
	keyDERBlock, _ := pem.Decode(cs.keyData)
	if keyDERBlock == nil || keyDERBlock.Type != "PRIVATE KEY" {
		return errors.New("CA key type mismatch")
	}
	cs.key, err = x509.ParsePKCS8PrivateKey(keyDERBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse CA key: %v", err)
	}

	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

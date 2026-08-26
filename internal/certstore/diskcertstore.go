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

// ErrNoSystemTrustStore signals that no system-wide certificate trust store exists
// on this device, e.g. on NixOS, where trust is configured declaratively.
// Init returns an error wrapping it after a successful NSS-only install; the store is
// fully initialized and usable in that case, but callers may want to inform the user
// that applications not backed by NSS will not trust the CA.
var ErrNoSystemTrustStore = errors.New("system trust store not found")

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

	// Seams for the platform-specific trust operations, bound to the real
	// implementations in NewDiskCertStore and substituted in tests.
	installTrustFn         func() error
	uninstallTrustFn       func() error
	installNSSFn           func(systemTrustMissing bool) error
	uninstallNSSFn         func() error
	systemTrustAvailableFn func() bool
	caTrustedBySystemFn    func() bool
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

	cs.installTrustFn = cs.installCATrust
	cs.uninstallTrustFn = cs.uninstallCATrust
	cs.installNSSFn = cs.installNSS
	cs.uninstallNSSFn = cs.uninstallNSS
	cs.systemTrustAvailableFn = systemTrustAvailable
	cs.caTrustedBySystemFn = cs.caTrustedBySystem

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

// Init loads the CA, creating and installing it into the trust stores first if needed.
// A non-nil error wrapping ErrNoSystemTrustStore signals success with a caveat:
// the store is fully initialized, but the CA is only trusted through NSS databases
// because the system has no trust store (e.g. NixOS). The caveat is reported on
// every call, not just the installing one, and clears once the CA shows up in the
// system certificate pool, e.g. after the user adds it to security.pki.certificateFiles.
func (cs *DiskCertStore) Init() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	systemTrustMissing := !cs.systemTrustAvailableFn()

	if cs.caStatusManager.GetCAInstalled() {
		if err := cs.loadCA(); err != nil {
			return fmt.Errorf("CA load: %w", err)
		}
		return cs.nssOnlyCaveat(systemTrustMissing)
	}

	// A CA on disk without the installed flag is left over from a failed install attempt.
	// Reuse it instead of regenerating, so trust established out of band
	// (e.g. NixOS security.pki.certificateFiles) survives retries.
	if err := cs.loadCA(); err != nil {
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
	}

	if !systemTrustMissing {
		if err := cs.installTrustFn(); err != nil {
			return fmt.Errorf("install CA to system trust store: %v", err)
		}
	}
	if err := cs.installNSSFn(systemTrustMissing); err != nil {
		if systemTrustMissing {
			// Without a system trust store, NSS is the only place the CA can be
			// installed; if that fails too, nothing on this system trusts it.
			// %v, not %w: wrapping ErrNoSystemTrustStore here would make total
			// failure look like the benign NSS-only fallback to callers.
			return fmt.Errorf("no system trust store found and NSS install failed: %v", err)
		}
		log.Printf("install CA to NSS database: %v", err)
	}
	cs.caStatusManager.SetCAInstalled(true)

	return cs.nssOnlyCaveat(systemTrustMissing)
}

// nssOnlyCaveat builds the "success with a caveat" error described on Init: the CA
// is trusted through NSS databases only. It is suppressed once the CA verifies
// against the system pool, i.e. the user established trust out of band.
func (cs *DiskCertStore) nssOnlyCaveat(systemTrustMissing bool) error {
	if !systemTrustMissing || cs.caTrustedBySystemFn() {
		return nil
	}
	return fmt.Errorf("CA installed to NSS databases only: %w", ErrNoSystemTrustStore)
}

// caTrustedBySystem reports whether the CA verifies against the system certificate
// pool, i.e. system-wide trust was established out of band, e.g. through
// security.pki.certificateFiles on NixOS. Go caches the pool per process, so a
// bundle updated mid-session is only noticed on the next app launch.
func (cs *DiskCertStore) caTrustedBySystem() bool {
	if cs.cert == nil {
		return false
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		return false
	}
	_, err = cs.cert.Verify(x509.VerifyOptions{Roots: pool})
	return err == nil
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

	if cs.systemTrustAvailableFn() {
		if err := cs.uninstallTrustFn(); err != nil {
			return fmt.Errorf("uninstall CA from system trust store: %w", err)
		}
	}
	if err := cs.uninstallNSSFn(); err != nil {
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

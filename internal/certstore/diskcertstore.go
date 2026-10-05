// Package certstore implements a certificate store.
package certstore

import (
	"crypto"
	"crypto/rand"
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
	"github.com/irbis-sh/zen-desktop/internal/config"
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
	// GetKeyStorage returns where the installed CA's key is stored. With no CA installed, it is
	// where the next CA's key will be stored.
	GetKeyStorage() config.KeyStorageType
}

// CA is the root CA as loaded by the store.
type CA struct {
	Cert *x509.Certificate
	Key  crypto.Signer
	// HardwareBacked tells certgen to sign leaves through an intermediate.
	HardwareBacked bool
}

// DiskCertStore is a disk-based certificate store.
// It manages the creation, loading, and installation of the root CA.
type DiskCertStore struct {
	mu              sync.RWMutex
	caStatusManager CAStatusManager
	folderPath      string
	certData        []byte
	certPath        string
	cert            *x509.Certificate
	keyPath         string
	hardwareKeyName string
	// key is the loaded CA key. A hardware key holds an open handle, which the store closes
	// before replacing or deleting the key.
	key     crypto.Signer
	orgName string

	// Seams for the platform-specific trust operations, bound to the real
	// implementations in NewDiskCertStore and substituted in tests.
	installTrustFn         func() error
	uninstallTrustFn       func() error
	installNSSFn           func(systemTrustMissing bool) error
	refreshNSSFn           func() error
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
	cs.hardwareKeyName = hardwareKeyName
	cs.orgName = orgName

	cs.installTrustFn = cs.installCATrust
	cs.uninstallTrustFn = cs.uninstallCATrust
	cs.installNSSFn = cs.installNSS
	cs.refreshNSSFn = cs.refreshNSS
	cs.uninstallNSSFn = cs.uninstallNSS
	cs.systemTrustAvailableFn = systemTrustAvailable
	cs.caTrustedBySystemFn = cs.caTrustedBySystem

	return cs, nil
}

// GetCA returns the root CA loaded by the last successful Init.
func (cs *DiskCertStore) GetCA() (CA, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.cert == nil || cs.key == nil {
		return CA{}, errors.New("CA not initialized")
	}

	return CA{Cert: cs.cert, Key: cs.key, HardwareBacked: isHardware(cs.key)}, nil
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
	backend := cs.keyBackend()

	if cs.caStatusManager.GetCAInstalled() {
		if err := cs.loadCA(backend); err != nil {
			return fmt.Errorf("CA load: %w", err)
		}
		if systemTrustMissing {
			// NSS databases are the CA's only trust path here, and they come and go:
			// Firefox creates one per profile on first launch. Without this, a browser
			// installed after the CA would never trust it.
			if err := cs.refreshNSSFn(); err != nil {
				log.Printf("refresh CA in NSS databases: %v", err)
			}
		}
		return cs.nssOnlyCaveat(systemTrustMissing)
	}

	// A CA on disk without the installed flag is left over from a failed install attempt.
	// Reuse it instead of regenerating, so trust established out of band
	// (e.g. NixOS security.pki.certificateFiles) survives retries.
	if err := cs.loadCA(backend); err != nil {
		if err := os.RemoveAll(cs.folderPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing CA folder: %v", err)
		}
		if err := os.MkdirAll(cs.folderPath, 0755); err != nil {
			return fmt.Errorf("create certs folder: %v", err)
		}
		if err := cs.newCA(backend); err != nil {
			return fmt.Errorf("create new CA: %w", err)
		}
		if err := cs.loadCA(backend); err != nil {
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

	// Removing trust only needs the certificate. Not loading the key lets a CA whose hardware
	// key is already gone still be uninstalled.
	if cs.cert == nil {
		if err := cs.loadCert(); err != nil {
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
	closeKey(cs.key)
	cs.key = nil
	if err := cs.keyBackend().Remove(); err != nil {
		return fmt.Errorf("remove CA key: %w", err)
	}
	if err := os.RemoveAll(cs.folderPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove CA folder: %w", err)
	}

	cs.cert, cs.certData = nil, nil
	cs.caStatusManager.SetCAInstalled(false)

	return nil
}

// DiscardKey deletes the key of a CA that is not installed, such as one left behind by a failed
// trust install. UninstallCA refuses to touch it, and a hardware key would otherwise outlive the
// switch to another storage, since Init only wipes the CA folder. A missing key is not an error.
func (cs *DiskCertStore) DiscardKey() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.caStatusManager.GetCAInstalled() {
		return errors.New("CA installed")
	}

	closeKey(cs.key)
	cs.key = nil
	return cs.keyBackend().Remove()
}

// keyBackend returns the backend for the configured key storage. Linux has no hardware key
// support yet and ignores the setting.
func (cs *DiskCertStore) keyBackend() keyBackend {
	if cs.caStatusManager.GetKeyStorage() == config.KeyStorageHardware && runtime.GOOS != "linux" {
		return hardwareKey{name: cs.hardwareKeyName}
	}
	return diskKey{path: cs.keyPath}
}

// newCA creates a new CA key and certificate. The certificate is saved to disk; where the key
// goes is up to the backend.
func (cs *DiskCertStore) newCA(backend keyBackend) error {
	priv, err := backend.Generate()
	if err != nil {
		return err
	}
	// loadCA opens the key again for use.
	defer closeKey(priv)
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
	}
	if isHardware(priv) {
		// A hardware root signs an intermediate, which signs the leaves.
		tpl.MaxPathLen = 1
	} else {
		tpl.MaxPathLenZero = true
	}

	cert, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
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
func (cs *DiskCertStore) loadCA(backend keyBackend) error {
	closeKey(cs.key)
	cs.key = nil
	if err := cs.loadCert(); err != nil {
		return err
	}
	key, err := backend.Load()
	if err != nil {
		return err
	}
	// Catches a certificate and a key that were created separately, e.g. a stale certificate
	// left next to a newer hardware key.
	if pub, ok := cs.cert.PublicKey.(interface{ Equal(crypto.PublicKey) bool }); !ok || !pub.Equal(key.Public()) {
		closeKey(key)
		return errors.New("CA key does not match the CA certificate")
	}
	cs.key = key
	return nil
}

// loadCert loads the existing CA certificate into memory.
func (cs *DiskCertStore) loadCert() error {
	certData, err := os.ReadFile(cs.certPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("CA cert does not exist at %s", cs.certPath)
	}
	if err != nil {
		return fmt.Errorf("read CA cert: %v", err)
	}
	certDERBlock, _ := pem.Decode(certData)
	if certDERBlock == nil || certDERBlock.Type != "CERTIFICATE" {
		return errors.New("CA cert type mismatch")
	}
	cert, err := x509.ParseCertificate(certDERBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse CA cert: %v", err)
	}
	cs.cert, cs.certData = cert, certData
	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

package certstore

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/hectane/go-acl"
	"github.com/irbis-sh/zen-desktop/internal/hwkey"
)

// keyBackend generates, opens and removes the root's private key.
type keyBackend interface {
	Generate() (crypto.Signer, error)
	Load() (crypto.Signer, error)
	Remove() error
}

// diskKey keeps an RSA-3072 key in a PEM file next to the certificate.
type diskKey struct {
	path string
}

func (k diskKey) Generate() (crypto.Signer, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	err = os.WriteFile(k.path, pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0600)
	if err != nil {
		return nil, fmt.Errorf("write private key at %s: %w", k.path, err)
	}
	if runtime.GOOS == "windows" {
		// 0600 to allow the current user to read/write/delete the file
		if err := acl.Chmod(k.path, 0600); err != nil {
			return nil, fmt.Errorf("chmod private key at %s: %w", k.path, err)
		}
	}

	return priv, nil
}

func (k diskKey) Load() (crypto.Signer, error) {
	keyData, err := os.ReadFile(k.path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("CA key does not exist at %s", k.path)
	}
	if err != nil {
		return nil, fmt.Errorf("read CA key: %w", err)
	}
	keyDERBlock, _ := pem.Decode(keyData)
	if keyDERBlock == nil || keyDERBlock.Type != "PRIVATE KEY" {
		return nil, errors.New("CA key type mismatch")
	}
	key, err := x509.ParsePKCS8PrivateKey(keyDERBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("CA key of type %T cannot sign", key)
	}
	return signer, nil
}

func (k diskKey) Remove() error {
	if err := os.Remove(k.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove CA key: %w", err)
	}
	return nil
}

// hardwareKey keeps a P-256 key in the Secure Enclave or the TPM. The signers it returns hold
// an open handle, which whoever keeps the signer must close.
type hardwareKey struct {
	name string
}

func (k hardwareKey) Generate() (crypto.Signer, error) {
	return hwkey.Create(k.name)
}

func (k hardwareKey) Load() (crypto.Signer, error) {
	signer, err := hwkey.Open(k.name)
	if errors.Is(err, hwkey.ErrNotFound) {
		return nil, fmt.Errorf("%w. The CA's hardware key is gone. Uninstall the CA in Settings and start Zen again to create a new one", err)
	}
	return signer, err
}

// Remove deletes the key. A key that is already gone is not an error, so a half-deleted CA can
// always be cleaned up.
func (k hardwareKey) Remove() error {
	err := hwkey.Delete(k.name)
	if errors.Is(err, hwkey.ErrNotFound) {
		log.Printf("hardware key already deleted: %v", err)
		return nil
	}
	if err != nil {
		return err
	}
	log.Println("hardware key deleted")
	return nil
}

// isHardware reports whether key is a handle to a hardware key.
func isHardware(key crypto.Signer) bool {
	_, ok := key.(hwkey.Signer)
	return ok
}

// closeKey releases the handle of a hardware key. Disk keys hold nothing to release.
func closeKey(key crypto.Signer) {
	if hk, ok := key.(hwkey.Signer); ok {
		hk.Close()
	}
}

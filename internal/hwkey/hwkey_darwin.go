package hwkey

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include "keychain_darwin.h"
*/
import "C"

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"
)

// errBufLen is the size of the buffer handed to the C layer for error messages.
const errBufLen = 256

const (
	errSecItemNotFound       = -25300
	errSecMissingEntitlement = -34018
)

// keychainSigner is a Signer backed by an ECDSA P-256 key in the keychain: a Secure Enclave key
// in release builds, a software key otherwise. The private key reference is held as an opaque
// pointer because the C layer hides SecKeyRef.
type keychainSigner struct {
	// mu keeps Close from releasing priv while Sign is using it.
	mu   sync.Mutex
	priv unsafe.Pointer // SecKeyRef
	pub  *ecdsa.PublicKey
}

func create(name string) (Signer, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	// Replace rather than add. The keychain enforces uniqueness on the public-key label, not on
	// the application tag (the name), so a second create would otherwise leave two keys under
	// one name, and Open would return an arbitrary one of them.
	if err := remove(name); err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("replace existing key: %w", err)
	}

	errBuf := make([]byte, errBufLen)
	var priv unsafe.Pointer
	ret := C.hwkey_create_key(cName, C.size_t(len(name)), &priv,
		(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(errBufLen)) //nolint:gocritic // cgo pointer checks trip dupSubExpr
	if ret != 0 {
		return nil, secError("create key", ret, errBuf)
	}

	// On any error the persisted key must be released and removed. On success the returned
	// signer owns the reference and releases it via Close.
	var success bool
	defer func() {
		if !success {
			C.hwkey_release(priv)
			_ = remove(name)
		}
	}()

	pub, err := copyPublicKey(priv)
	if err != nil {
		return nil, err
	}

	success = true
	return &keychainSigner{priv: priv, pub: pub}, nil
}

func open(name string) (Signer, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	errBuf := make([]byte, errBufLen)
	var priv unsafe.Pointer
	ret := C.hwkey_load_key(cName, C.size_t(len(name)), &priv,
		(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(errBufLen)) //nolint:gocritic // cgo pointer checks trip dupSubExpr
	if ret != 0 {
		return nil, secError("open key", ret, errBuf)
	}

	var success bool
	defer func() {
		if !success {
			C.hwkey_release(priv)
		}
	}()

	pub, err := copyPublicKey(priv)
	if err != nil {
		return nil, err
	}

	success = true
	return &keychainSigner{priv: priv, pub: pub}, nil
}

func remove(name string) error {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	errBuf := make([]byte, errBufLen)
	ret := C.hwkey_delete_key(cName, C.size_t(len(name)),
		(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(errBufLen))
	if ret != 0 {
		return secError("delete key", ret, errBuf)
	}
	return nil
}

// copyPublicKey exports the public key of a private key reference and parses it. The export is
// an ANSI X9.63 uncompressed P-256 point (0x04 || X || Y).
func copyPublicKey(priv unsafe.Pointer) (*ecdsa.PublicKey, error) {
	errBuf := make([]byte, errBufLen)
	var buf *C.uint8_t
	var n C.size_t

	ret := C.hwkey_copy_public_key(priv, &buf, &n,
		(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(errBufLen)) //nolint:gocritic // cgo pointer checks trip dupSubExpr
	if ret != 0 {
		return nil, secError("copy public key", ret, errBuf)
	}
	defer C.free(unsafe.Pointer(buf))

	raw := C.GoBytes(unsafe.Pointer(buf), C.int(n))
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
	if err != nil {
		return nil, fmt.Errorf("%s: parse public key: %w", backendName, err)
	}
	return pub, nil
}

func (s *keychainSigner) Public() crypto.PublicKey {
	return s.pub
}

// Sign implements crypto.Signer using ECDSA over the keychain key.
func (s *keychainSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.priv == nil {
		return nil, fmt.Errorf("%s: signer is closed", backendName)
	}
	// x509.CreateCertificate, the only caller, always hashes with SHA-256 for a P-256 key.
	if opts.HashFunc() != crypto.SHA256 || len(digest) != sha256.Size {
		return nil, fmt.Errorf("%s: want a SHA-256 digest, got %d bytes of %v", backendName, len(digest), opts.HashFunc())
	}

	errBuf := make([]byte, errBufLen)
	var sig *C.uint8_t
	var n C.size_t

	ret := C.hwkey_sign(s.priv,
		(*C.uint8_t)(unsafe.Pointer(&digest[0])), C.size_t(len(digest)),
		&sig, &n,
		(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(errBufLen)) //nolint:gocritic // cgo pointer checks trip dupSubExpr
	if ret != 0 {
		return nil, secError("sign", ret, errBuf)
	}
	defer C.free(unsafe.Pointer(sig))

	return C.GoBytes(unsafe.Pointer(sig), C.int(n)), nil
}

func (s *keychainSigner) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.priv != nil {
		C.hwkey_release(s.priv)
		s.priv = nil
	}
	return nil
}

// secError turns a C helper's return code and error buffer into a statusError, for example
// "secure enclave: open key: -25300 errSecItemNotFound".
func secError(op string, ret C.int, errBuf []byte) error {
	code := int(ret)
	msg := cString(errBuf)
	name := secStatusName(code)

	var detail string
	switch {
	case msg != "" && name != "":
		detail = fmt.Sprintf("%d %s (%s)", code, name, msg)
	case name != "":
		detail = fmt.Sprintf("%d %s", code, name)
	case msg != "" && code != -1:
		detail = fmt.Sprintf("%d (%s)", code, msg)
	case msg != "":
		detail = msg
	default:
		detail = fmt.Sprintf("status %d", code)
	}

	text := fmt.Sprintf("%s: %s: %s", backendName, op, detail)
	if code == errSecMissingEntitlement {
		// Only a build without the provisioning profile gets here, which is a packaging bug.
		text += ". This build of Zen cannot use the Secure Enclave"
	}
	return &statusError{msg: text, notFound: code == errSecItemNotFound}
}

// secStatusName maps the OSStatus codes the backend is likely to surface to their symbolic
// names. It returns "" for anything else.
func secStatusName(code int) string {
	switch code {
	case -4:
		return "errSecUnimplemented"
	case -50:
		return "errSecParam"
	case -128:
		return "errSecUserCanceled"
	case -25291:
		return "errSecNotAvailable"
	case -25293:
		return "errSecAuthFailed"
	case -25299:
		return "errSecDuplicateItem"
	case errSecItemNotFound:
		return "errSecItemNotFound"
	case -25308:
		return "errSecInteractionNotAllowed"
	case errSecMissingEntitlement:
		return "errSecMissingEntitlement"
	default:
		return ""
	}
}

// cString returns the NUL-terminated prefix of a C-filled byte buffer as a Go string.
func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

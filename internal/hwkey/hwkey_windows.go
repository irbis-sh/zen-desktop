package hwkey

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	nCryptECDSAP256Algorithm   = "ECDSA_P256"
	bCryptECDSAP256PublicMagic = 0x31534345 // BCRYPT_ECDSA_PUBLIC_P256_MAGIC ("ECS1")
	bCryptECCPublicBlob        = "ECCPUBLICBLOB"

	nCryptKeyUsageProperty = "Key Usage"

	nCryptAllowSigningFlag = 0x00000002
	nCryptOverwriteKeyFlag = 0x00000080

	p256CoordinateLength = 32
)

const (
	nteBadKeyset       = 0x80090016
	nteDeviceNotReady  = 0x80090030
	nteDeviceNotFound  = 0x80090035
	nteNotSupported    = 0x80090029
	nteExists          = 0x8009000F
	ntePerm            = 0x80090010
	nteBadFlags        = 0x80090009
	nteInvalidParam    = 0x80090027
	nteInvalidHandle   = 0x80090026
	nteFail            = 0x80090020
	tbsETPMNotFound    = 0x8028400F
	tbsEServiceStopped = 0x80284008
)

type nCryptProvHandle uintptr
type nCryptKeyHandle uintptr

func create(name string) (Signer, error) {
	hProv, err := openProvider()
	if err != nil {
		return nil, err
	}
	// On any error both handles must be freed. On success they stay alive because the returned
	// signer owns them and releases them via Close.
	var success bool
	defer func() {
		if !success {
			nCryptFreeObject(uintptr(hProv))
		}
	}()

	algID, err := windows.UTF16PtrFromString(nCryptECDSAP256Algorithm)
	if err != nil {
		return nil, fmt.Errorf("%s: convert algorithm id: %w", backendName, err)
	}
	keyName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("%s: convert key name: %w", backendName, err)
	}

	var hKey nCryptKeyHandle
	ret := nCryptCreatePersistedKey(hProv, &hKey, algID, keyName, 0, nCryptOverwriteKeyFlag)
	if ret != 0 {
		return nil, ncryptError("create persisted key", ret)
	}
	var finalized bool
	defer func() {
		if success {
			return
		}
		// A finalised key is already persisted. NCryptDeleteKey frees the handle on success.
		if finalized && nCryptDeleteKey(hKey, 0) == 0 {
			return
		}
		nCryptFreeObject(uintptr(hKey))
	}()

	usage := uint32(nCryptAllowSigningFlag)
	usageProp, err := windows.UTF16PtrFromString(nCryptKeyUsageProperty)
	if err != nil {
		return nil, fmt.Errorf("%s: convert key usage property: %w", backendName, err)
	}
	ret = nCryptSetProperty(hKey, usageProp,
		(*byte)(unsafe.Pointer(&usage)), uint32(unsafe.Sizeof(usage)), 0)
	if ret != 0 {
		return nil, ncryptError("set key usage", ret)
	}

	ret = nCryptFinalizeKey(hKey, 0)
	if ret != 0 {
		return nil, ncryptError("finalize key", ret)
	}
	finalized = true

	pub, err := exportECCPublicKey(hKey)
	if err != nil {
		return nil, err
	}

	success = true
	return &ncryptSigner{hProv: hProv, hKey: hKey, pub: pub}, nil
}

func open(name string) (Signer, error) {
	hProv, err := openProvider()
	if err != nil {
		return nil, err
	}
	var success bool
	defer func() {
		if !success {
			nCryptFreeObject(uintptr(hProv))
		}
	}()

	keyName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("%s: convert key name: %w", backendName, err)
	}

	var hKey nCryptKeyHandle
	ret := nCryptOpenKey(hProv, &hKey, keyName, 0, 0)
	if ret != 0 {
		return nil, ncryptError("open key", ret)
	}
	defer func() {
		if !success {
			nCryptFreeObject(uintptr(hKey))
		}
	}()

	pub, err := exportECCPublicKey(hKey)
	if err != nil {
		return nil, err
	}

	success = true
	return &ncryptSigner{hProv: hProv, hKey: hKey, pub: pub}, nil
}

func remove(name string) error {
	hProv, err := openProvider()
	if err != nil {
		return err
	}
	defer nCryptFreeObject(uintptr(hProv))

	keyName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("%s: convert key name: %w", backendName, err)
	}

	var hKey nCryptKeyHandle
	ret := nCryptOpenKey(hProv, &hKey, keyName, 0, 0)
	if ret != 0 {
		return ncryptError("open key", ret)
	}

	// NCryptDeleteKey frees hKey on success, so it is only freed here on failure.
	ret = nCryptDeleteKey(hKey, 0)
	if ret != 0 {
		nCryptFreeObject(uintptr(hKey))
		return ncryptError("delete key", ret)
	}
	return nil
}

func openProvider() (nCryptProvHandle, error) {
	provName, err := windows.UTF16PtrFromString(providerName)
	if err != nil {
		return 0, fmt.Errorf("%s: convert provider name: %w", backendName, err)
	}

	var hProv nCryptProvHandle
	// No NCRYPT_MACHINE_KEY_FLAG: keys live in the per-user store, so no elevation is needed.
	ret := nCryptOpenStorageProvider(&hProv, provName, 0)
	if ret != 0 {
		return 0, ncryptError("open storage provider", ret)
	}
	return hProv, nil
}

func exportECCPublicKey(hKey nCryptKeyHandle) (*ecdsa.PublicKey, error) {
	blobType, err := windows.UTF16PtrFromString(bCryptECCPublicBlob)
	if err != nil {
		return nil, fmt.Errorf("%s: convert blob type: %w", backendName, err)
	}

	var cb uint32
	ret := nCryptExportKey(hKey, 0, blobType, nil, nil, 0, &cb, 0)
	if ret != 0 {
		return nil, ncryptError("export key (size)", ret)
	}
	buf := make([]byte, cb)
	ret = nCryptExportKey(hKey, 0, blobType, nil, &buf[0], cb, &cb, 0)
	if ret != 0 {
		return nil, ncryptError("export key", ret)
	}
	pub, err := parseECCPublicBlob(buf[:cb])
	if err != nil {
		return nil, fmt.Errorf("%s: parse ECC public blob: %w", backendName, err)
	}
	return pub, nil
}

type ncryptSigner struct {
	// mu keeps Close from freeing the handles while Sign is using them.
	mu    sync.Mutex
	hProv nCryptProvHandle
	hKey  nCryptKeyHandle
	pub   *ecdsa.PublicKey
}

func (s *ncryptSigner) Public() crypto.PublicKey {
	return s.pub
}

// Sign implements crypto.Signer using ECDSA over the NCrypt key. NCryptSignHash returns the
// signature as raw r || s coordinates; it is converted to the ASN.1 DER SEQUENCE{r,s} that
// x509.CreateCertificate expects from an ECDSA signer.
func (s *ncryptSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hKey == 0 {
		return nil, fmt.Errorf("%s: signer is closed", backendName)
	}
	// x509.CreateCertificate, the only caller, always hashes with SHA-256 for a P-256 key.
	if opts.HashFunc() != crypto.SHA256 || len(digest) != sha256.Size {
		return nil, fmt.Errorf("%s: want a SHA-256 digest, got %d bytes of %v", backendName, len(digest), opts.HashFunc())
	}

	digestLen := uint32(sha256.Size)
	var cb uint32
	ret := nCryptSignHash(s.hKey, nil, &digest[0], digestLen, nil, 0, &cb, 0)
	if ret != 0 {
		return nil, ncryptError("sign hash (size)", ret)
	}
	sig := make([]byte, cb)
	ret = nCryptSignHash(s.hKey, nil, &digest[0], digestLen, &sig[0], cb, &cb, 0)
	if ret != 0 {
		return nil, ncryptError("sign hash", ret)
	}
	sig = sig[:cb]

	if len(sig) != 2*p256CoordinateLength {
		return nil, fmt.Errorf("%s: unexpected raw signature length %d", backendName, len(sig))
	}
	r := new(big.Int).SetBytes(sig[:p256CoordinateLength])
	sVal := new(big.Int).SetBytes(sig[p256CoordinateLength:])
	der, err := asn1.Marshal(struct{ R, S *big.Int }{r, sVal})
	if err != nil {
		return nil, fmt.Errorf("%s: encode signature: %w", backendName, err)
	}
	return der, nil
}

func (s *ncryptSigner) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hKey != 0 {
		nCryptFreeObject(uintptr(s.hKey))
		s.hKey = 0
	}
	if s.hProv != 0 {
		nCryptFreeObject(uintptr(s.hProv))
		s.hProv = 0
	}
	return nil
}

// parseECCPublicBlob parses a BCRYPT_ECCKEY_BLOB header (magic and coordinate length, both
// little-endian uint32) followed by the X and Y coordinates.
// See: https://learn.microsoft.com/en-us/windows/win32/api/bcrypt/ns-bcrypt-bcrypt_ecckey_blob.
func parseECCPublicBlob(blob []byte) (*ecdsa.PublicKey, error) {
	const hdrLen = 8
	if len(blob) < hdrLen {
		return nil, fmt.Errorf("blob too short: %d bytes", len(blob))
	}
	magic := binary.LittleEndian.Uint32(blob[0:4])
	keyLen := int(binary.LittleEndian.Uint32(blob[4:8]))
	if magic != bCryptECDSAP256PublicMagic {
		return nil, fmt.Errorf("unexpected magic: 0x%08x", magic)
	}
	if keyLen != p256CoordinateLength {
		return nil, fmt.Errorf("unexpected coordinate length %d for P-256", keyLen)
	}
	if len(blob) < hdrLen+2*keyLen {
		return nil, fmt.Errorf("blob truncated: need %d, have %d", hdrLen+2*keyLen, len(blob))
	}

	// Rebuild the uncompressed point (0x04 || X || Y) so the parser checks it is on the curve.
	point := append([]byte{4}, blob[hdrLen:hdrLen+2*keyLen]...)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
	if err != nil {
		return nil, errors.New("invalid P-256 point")
	}
	return pub, nil
}

// ncryptError turns an NCrypt status into a statusError, for example
// "tpm: create persisted key: 0x80090030 NTE_DEVICE_NOT_READY".
func ncryptError(op string, ret uint32) error {
	msg := fmt.Sprintf("%s: %s: 0x%08x", backendName, op, ret)
	if name := ncryptStatusName(ret); name != "" {
		msg += " " + name
	}
	return &statusError{msg: msg, notFound: ret == nteBadKeyset}
}

// ncryptStatusName maps the status codes the backend is likely to surface to their symbolic
// names. It returns "" for anything else.
func ncryptStatusName(code uint32) string {
	switch code {
	case nteBadKeyset:
		return "NTE_BAD_KEYSET"
	case nteDeviceNotReady:
		return "NTE_DEVICE_NOT_READY"
	case nteDeviceNotFound:
		return "NTE_DEVICE_NOT_FOUND"
	case nteNotSupported:
		return "NTE_NOT_SUPPORTED"
	case nteExists:
		return "NTE_EXISTS"
	case ntePerm:
		return "NTE_PERM"
	case nteBadFlags:
		return "NTE_BAD_FLAGS"
	case nteInvalidParam:
		return "NTE_INVALID_PARAMETER"
	case nteInvalidHandle:
		return "NTE_INVALID_HANDLE"
	case nteFail:
		return "NTE_FAIL"
	case tbsETPMNotFound:
		return "TBS_E_TPM_NOT_FOUND"
	case tbsEServiceStopped:
		return "TBS_E_SERVICE_NOT_RUNNING"
	default:
		return ""
	}
}

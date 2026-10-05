// Package hwkey keeps ECDSA P-256 signing keys in the device's key hardware: the Secure Enclave
// on macOS and the TPM on Windows. The private key is generated inside the hardware and cannot be
// read out; callers hold a handle and can only ask for signatures.
//
// Release builds (-tags prod) use the hardware. Other builds use a software key in the same
// platform key store, because the Enclave path only works in a signed, bundled app and the TPM
// path needs a TPM. Everything else, including key names and error mapping, is shared, so a
// development build exercises the same code apart from the attributes that select the hardware.
//
// Linux is not supported yet: every function returns an error.
package hwkey

import (
	"crypto"
	"errors"
)

// ErrNotFound is returned by Open and Delete when no key with the given name exists.
var ErrNotFound = errors.New("hwkey: key not found")

// Signer is a handle to a private key that never leaves the hardware.
//
// Handles are safe for concurrent use. Close waits for a Sign in progress, and Sign after Close
// returns an error, so a handle can be closed while another goroutine may still sign with it.
type Signer interface {
	crypto.Signer // Public() is *ecdsa.PublicKey; Sign returns ASN.1 DER.
	Close() error
}

// Available reports whether this device can hold a key. A nil error means yes.
// The error is user-facing: it is shown as the tooltip on the disabled switch.
func Available() error {
	return available()
}

// Create generates a new P-256 key in hardware under name. It replaces any existing key with
// that name.
func Create(name string) (Signer, error) {
	return create(name)
}

// Open returns a handle to an existing key. Missing keys return ErrNotFound.
func Open(name string) (Signer, error) {
	return open(name)
}

// Delete removes the key. A missing key returns ErrNotFound.
func Delete(name string) error {
	return remove(name)
}

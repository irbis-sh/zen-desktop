//go:build !prod

package hwkey

// The Secure Enclave path only works in a signed app bundle that carries a provisioning profile;
// macOS kills a bare binary that claims the entitlements it needs. Development builds therefore
// keep a software key in the login keychain. The login keychain ties an item's ACL to the
// binary that created it, so a rebuilt binary gets a keychain prompt the first time it opens
// the key.

const backendName = "software keychain"

func available() error {
	return nil
}

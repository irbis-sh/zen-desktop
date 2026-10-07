#ifndef ZEN_HWKEY_KEYCHAIN_DARWIN_H
#define ZEN_HWKEY_KEYCHAIN_DARWIN_H

#include <stddef.h>
#include <stdint.h>

// Thin C layer over the macOS Security framework for an ECDSA P-256 signing key. With
// ZEN_SECURE_ENCLAVE defined the key lives in the Secure Enclave; without it, it is a software
// key in the login keychain.
//
// Every function returns 0 on success. On failure it returns a non-zero status: the OSStatus
// reported by the Security framework where there is one, otherwise -1. A human-readable
// message, if any, is written into err (NUL-terminated, up to errLen).
// Key references are handed back to Go as opaque void* (SecKeyRef); release with
// hwkey_release. Output byte buffers are malloc'd and must be freed by the caller.

// hwkey_probe_enclave creates a non-permanent Secure Enclave key and discards it. It checks for
// the hardware without touching the keychain, so it needs no entitlements.
int hwkey_probe_enclave(char *err, size_t errLen);

// hwkey_create_key creates a permanent ECDSA P-256 key tagged with the given name and returns
// its private key reference in outPrivRef.
int hwkey_create_key(const char *tag, size_t tagLen, void **outPrivRef, char *err, size_t errLen);

// hwkey_load_key looks up an existing key by tag and returns its private key reference.
// Returns errSecItemNotFound (-25300) when no key matches the tag.
int hwkey_load_key(const char *tag, size_t tagLen, void **outPrivRef, char *err, size_t errLen);

// hwkey_delete_key removes every key identified by tag from the keychain.
// Returns errSecItemNotFound (-25300) when no key matches the tag.
int hwkey_delete_key(const char *tag, size_t tagLen, char *err, size_t errLen);

// hwkey_copy_public_key exports the public key of privRef in ANSI X9.63 form
// (0x04 || X || Y, 65 bytes for P-256) into a malloc'd buffer.
int hwkey_copy_public_key(void *privRef, uint8_t **outBuf, size_t *outLen, char *err, size_t errLen);

// hwkey_sign signs a SHA-256 digest with privRef using ECDSA. The output is an ASN.1 DER
// SEQUENCE{r, s}.
int hwkey_sign(void *privRef, const uint8_t *digest, size_t digestLen,
               uint8_t **outSig, size_t *outLen, char *err, size_t errLen);

// hwkey_release releases a key reference returned by hwkey_create_key / hwkey_load_key.
void hwkey_release(void *ref);

#endif

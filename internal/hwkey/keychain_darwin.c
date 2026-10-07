#include "keychain_darwin.h"

#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

// set_err writes a CFString message into the caller's error buffer.
static void set_err(char *err, size_t errLen, CFStringRef msg) {
    if (err == NULL || errLen == 0) {
        return;
    }
    if (msg == NULL || !CFStringGetCString(msg, err, (CFIndex)errLen, kCFStringEncodingUTF8)) {
        err[0] = '\0';
    }
}

// fail_cferror renders a CFErrorRef's description into the caller's error buffer, releases the
// error and returns the status to hand back to Go. Keychain failures arrive as CFErrors in the
// OSStatus domain; their code is the OSStatus, which is what Go maps to a name. Anything else
// becomes -1.
static int fail_cferror(char *err, size_t errLen, CFErrorRef cfErr) {
    if (cfErr == NULL) {
        set_err(err, errLen, NULL);
        return -1;
    }
    CFStringRef desc = CFErrorCopyDescription(cfErr);
    set_err(err, errLen, desc);
    if (desc != NULL) {
        CFRelease(desc);
    }
    int status = -1;
    CFErrorDomain domain = CFErrorGetDomain(cfErr);
    if (domain != NULL && CFEqual(domain, kCFErrorDomainOSStatus) && CFErrorGetCode(cfErr) != 0) {
        status = (int)CFErrorGetCode(cfErr);
    }
    CFRelease(cfErr);
    return status;
}

static CFDataRef make_tag(const char *tag, size_t tagLen) {
    return CFDataCreate(kCFAllocatorDefault, (const UInt8 *)tag, (CFIndex)tagLen);
}

// make_enclave_access returns the access control for an Enclave key. The CA key signs
// unattended, so private-key usage has no user-presence (biometric/passcode) requirement.
// "After first unlock" is the strictest class that still lets Zen sign after an autostart at
// login and while the screen is locked. ThisDeviceOnly keeps the key out of backups.
static SecAccessControlRef make_enclave_access(CFErrorRef *cfErr) {
    return SecAccessControlCreateWithFlags(
        kCFAllocatorDefault,
        kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        kSecAccessControlPrivateKeyUsage,
        cfErr);
}

// make_query builds the dictionary that identifies a key by tag, shared by load and delete. The
// same attributes are used at creation time so lookups stay consistent.
static CFMutableDictionaryRef make_query(const char *tag, size_t tagLen) {
    CFMutableDictionaryRef q = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(q, kSecClass, kSecClassKey);
    CFDictionarySetValue(q, kSecAttrKeyClass, kSecAttrKeyClassPrivate);
    CFDictionarySetValue(q, kSecAttrKeyType, kSecAttrKeyTypeECSECPrimeRandom);
#ifdef ZEN_SECURE_ENCLAVE
    CFDictionarySetValue(q, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave);
    CFDictionarySetValue(q, kSecUseDataProtectionKeychain, kCFBooleanTrue);
#endif
    CFDataRef tagData = make_tag(tag, tagLen);
    CFDictionarySetValue(q, kSecAttrApplicationTag, tagData);
    CFRelease(tagData);
    return q;
}

// copy_to_buf copies a CFData's bytes into a freshly malloc'd buffer for the caller.
static int copy_to_buf(CFDataRef data, uint8_t **outBuf, size_t *outLen, char *err, size_t errLen) {
    CFIndex len = CFDataGetLength(data);
    // Avoid malloc(0), which may return NULL and be misreported as an allocation failure.
    uint8_t *buf = (uint8_t *)malloc(len > 0 ? (size_t)len : 1);
    if (buf == NULL) {
        set_err(err, errLen, CFSTR("malloc failed"));
        return -1;
    }
    memcpy(buf, CFDataGetBytePtr(data), (size_t)len);
    *outBuf = buf;
    *outLen = (size_t)len;
    return 0;
}

int hwkey_probe_enclave(char *err, size_t errLen) {
    CFErrorRef cfErr = NULL;
    SecAccessControlRef access = make_enclave_access(&cfErr);
    if (access == NULL) {
        return fail_cferror(err, errLen, cfErr);
    }

    CFMutableDictionaryRef privAttrs = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(privAttrs, kSecAttrIsPermanent, kCFBooleanFalse);
    CFDictionarySetValue(privAttrs, kSecAttrAccessControl, access);

    CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(attrs, kSecAttrKeyType, kSecAttrKeyTypeECSECPrimeRandom);
    int bits = 256;
    CFNumberRef bitsNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &bits);
    CFDictionarySetValue(attrs, kSecAttrKeySizeInBits, bitsNum);
    CFDictionarySetValue(attrs, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave);
    CFDictionarySetValue(attrs, kSecPrivateKeyAttrs, privAttrs);

    SecKeyRef privKey = SecKeyCreateRandomKey(attrs, &cfErr);

    CFRelease(bitsNum);
    CFRelease(attrs);
    CFRelease(privAttrs);
    CFRelease(access);

    if (privKey == NULL) {
        return fail_cferror(err, errLen, cfErr);
    }
    CFRelease(privKey);
    return 0;
}

int hwkey_create_key(const char *tag, size_t tagLen, void **outPrivRef, char *err, size_t errLen) {
    CFErrorRef cfErr = NULL;

    CFDataRef tagData = make_tag(tag, tagLen);

    CFMutableDictionaryRef privAttrs = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(privAttrs, kSecAttrIsPermanent, kCFBooleanTrue);
    CFDictionarySetValue(privAttrs, kSecAttrApplicationTag, tagData);

#ifdef ZEN_SECURE_ENCLAVE
    SecAccessControlRef access = make_enclave_access(&cfErr);
    if (access == NULL) {
        CFRelease(privAttrs);
        CFRelease(tagData);
        return fail_cferror(err, errLen, cfErr);
    }
    CFDictionarySetValue(privAttrs, kSecAttrAccessControl, access);
#endif

    CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(attrs, kSecAttrKeyType, kSecAttrKeyTypeECSECPrimeRandom);
    int bits = 256;
    CFNumberRef bitsNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &bits);
    CFDictionarySetValue(attrs, kSecAttrKeySizeInBits, bitsNum);
#ifdef ZEN_SECURE_ENCLAVE
    // Hardware-backed key in the data protection keychain. Without these two attributes the key
    // is an ordinary software key in the login keychain, which is how development builds
    // exercise this code without the keychain-access-groups entitlement or a signing identity.
    CFDictionarySetValue(attrs, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave);
    CFDictionarySetValue(attrs, kSecUseDataProtectionKeychain, kCFBooleanTrue);
#endif
    CFDictionarySetValue(attrs, kSecPrivateKeyAttrs, privAttrs);

    SecKeyRef privKey = SecKeyCreateRandomKey(attrs, &cfErr);

    CFRelease(bitsNum);
    CFRelease(attrs);
    CFRelease(privAttrs);
    CFRelease(tagData);
#ifdef ZEN_SECURE_ENCLAVE
    CFRelease(access);
#endif

    if (privKey == NULL) {
        return fail_cferror(err, errLen, cfErr);
    }

    *outPrivRef = (void *)privKey;
    return 0;
}

int hwkey_load_key(const char *tag, size_t tagLen, void **outPrivRef, char *err, size_t errLen) {
    CFMutableDictionaryRef q = make_query(tag, tagLen);
    CFDictionarySetValue(q, kSecReturnRef, kCFBooleanTrue);

    CFTypeRef result = NULL;
    OSStatus st = SecItemCopyMatching(q, &result);
    CFRelease(q);

    if (st != errSecSuccess) {
        set_err(err, errLen, NULL);
        return (int)st;
    }

    *outPrivRef = (void *)result; // SecKeyRef, retained; released via hwkey_release.
    return 0;
}

int hwkey_delete_key(const char *tag, size_t tagLen, char *err, size_t errLen) {
    CFMutableDictionaryRef q = make_query(tag, tagLen);
    OSStatus st = SecItemDelete(q);
    CFRelease(q);

    if (st != errSecSuccess) {
        set_err(err, errLen, NULL);
        return (int)st;
    }
    return 0;
}

int hwkey_copy_public_key(void *privRef, uint8_t **outBuf, size_t *outLen, char *err, size_t errLen) {
    SecKeyRef pub = SecKeyCopyPublicKey((SecKeyRef)privRef);
    if (pub == NULL) {
        set_err(err, errLen, CFSTR("SecKeyCopyPublicKey returned NULL"));
        return -1;
    }

    CFErrorRef cfErr = NULL;
    CFDataRef data = SecKeyCopyExternalRepresentation(pub, &cfErr);
    CFRelease(pub);
    if (data == NULL) {
        return fail_cferror(err, errLen, cfErr);
    }

    int rc = copy_to_buf(data, outBuf, outLen, err, errLen);
    CFRelease(data);
    return rc;
}

int hwkey_sign(void *privRef, const uint8_t *digest, size_t digestLen,
               uint8_t **outSig, size_t *outLen, char *err, size_t errLen) {
    CFDataRef digestData = CFDataCreate(kCFAllocatorDefault, digest, (CFIndex)digestLen);
    CFErrorRef cfErr = NULL;
    CFDataRef sig = SecKeyCreateSignature((SecKeyRef)privRef, kSecKeyAlgorithmECDSASignatureDigestX962SHA256,
                                          digestData, &cfErr);
    CFRelease(digestData);
    if (sig == NULL) {
        return fail_cferror(err, errLen, cfErr);
    }

    int rc = copy_to_buf(sig, outSig, outLen, err, errLen);
    CFRelease(sig);
    return rc;
}

void hwkey_release(void *ref) {
    if (ref != NULL) {
        CFRelease((CFTypeRef)ref);
    }
}

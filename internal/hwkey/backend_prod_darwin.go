//go:build prod

package hwkey

/*
// cgo applies this flag to every C file in the package, so it is what puts keychain_darwin.c's
// keys in the Secure Enclave. Without it, a release build silently uses software keys.
#cgo CFLAGS: -DZEN_SECURE_ENCLAVE
#include "keychain_darwin.h"
*/
import "C"

import (
	"errors"
	"log"
	"unsafe"
)

const backendName = "secure enclave"

func available() error {
	if err := probeEnclave(); err != nil {
		log.Printf("hardware key unavailable: %v", err)
		return errors.New("no Secure Enclave")
	}
	return nil
}

// probeEnclave checks for a Secure Enclave by creating a throwaway key that is never stored.
func probeEnclave() error {
	errBuf := make([]byte, errBufLen)
	ret := C.hwkey_probe_enclave((*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(errBufLen))
	if ret != 0 {
		return secError("probe", ret, errBuf)
	}
	return nil
}

//go:build windows

package proxy

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// Real network errors on Windows wrap the WSA errnos; the syscall constants are
// synthetic APPLICATION_ERROR-based values that errors.Is never matches against
// them. Both are listed so that hand-constructed errors (tests, other packages)
// classify the same on every platform.
var (
	connRefusedErrs = []error{windows.WSAECONNREFUSED, syscall.ECONNREFUSED}
	connResetErrs   = []error{windows.WSAECONNRESET, syscall.ECONNRESET}
)

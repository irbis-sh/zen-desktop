//go:build !windows

package proxy

import "syscall"

var (
	connRefusedErrs = []error{syscall.ECONNREFUSED}
	connResetErrs   = []error{syscall.ECONNRESET}
)

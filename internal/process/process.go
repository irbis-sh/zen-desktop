// Package process provides the process information Zen needs for local HTTP requests.
package process

import "errors"

// PID identifies an operating system process.
type PID int

// Info describes the process that owns a local request connection.
type Info struct {
	PID            PID
	ExecutablePath string
}

var (
	// ErrNotFound is returned when no matching process is found.
	ErrNotFound = errors.New("process not found")
)

// Name returns a best-effort display name for the process.
//
// Its meaning is operating system dependent. The returned name may come from
// process metadata rather than the executable filename, may be truncated, and
// is not guaranteed to be stable over the lifetime of the process.
//
// Name is intended for display and diagnostics. Callers that need a stable
// identifier should use PID or ExecutablePath instead.
func (info Info) Name() (string, error) {
	if info.PID == 0 {
		return "", ErrNotFound
	}
	return pidName(info.PID, info.ExecutablePath)
}

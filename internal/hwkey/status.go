//go:build darwin || windows

package hwkey

// statusError is a failure reported by the platform key store. It carries the backend name and
// the platform's status code in its message, and matches ErrNotFound when the platform said the
// key does not exist.
type statusError struct {
	msg      string
	notFound bool
}

func (e *statusError) Error() string { return e.msg }

func (e *statusError) Is(target error) bool { return e.notFound && target == ErrNotFound }

//go:build !darwin

package netmon

// readState reports an empty state: this system has no reader yet. The monitor still
// reports wakes from sleep.
func readState() (State, error) {
	return State{}, nil
}

// watch reports nothing: this system has no watcher yet.
func watch(func()) (func(), error) {
	return func() {}, nil
}

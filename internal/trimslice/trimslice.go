package trimslice

// TrimSlice returns a slice with the same elements as s, and no spare capacity.
// If s already has capacity equal to its length, TrimSlice returns s unchanged.
//
// Use it for long-lived slices after appends are complete to let the GC reclaim
// unused backing-array storage.
func TrimSlice[T any](s []T) []T {
	if len(s) == cap(s) {
		return s
	}

	newS := make([]T, len(s))
	copy(newS, s)
	return newS
}

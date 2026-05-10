package byteset

// Set is a set of bytes (0-255) implemented as a bitmap.
type Set [4]uint64

func (s *Set) Add(b byte) {
	s[b/64] |= 1 << (b % 64)
}

func (s *Set) Has(b byte) bool {
	return s[b/64]&(1<<(b%64)) != 0
}

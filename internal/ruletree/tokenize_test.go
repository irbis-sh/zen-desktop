package ruletree

import (
	"testing"
	"unsafe"
)

func TestTokenize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		s    string
		want string
	}{
		{"abc123", "abc123"},
		{"a*b^c|", "a*b^c|"},
		{"**", "*"},
		{"a**b", "a*b"},
		{"***a***", "*a*"},
	}

	for _, test := range tests {
		if got := tokenize(test.s); got != test.want {
			t.Errorf("tokenize(%q) = %q, want %q", test.s, got, test.want)
		}
	}
}

func TestTokenizeSharesMemory(t *testing.T) {
	t.Parallel()

	s := "example.com/ads/*^|"
	if got := tokenize(s); unsafe.StringData(got) != unsafe.StringData(s) {
		t.Error("tokenize copied a pattern without a run of wildcards")
	}
}

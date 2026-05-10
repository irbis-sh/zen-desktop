package scriptlet

import (
	"reflect"
	"testing"
)

func TestArgSplit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    string
		expected []string
	}{
		{"", []string{}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a ,  b  , c ", []string{"a", "b", "c"}},
		{`"a, b",c`, []string{`"a, b"`, "c"}},
		{`'a, b', c`, []string{"'a, b'", "c"}},
		{`a\,b,c`, []string{`a\,b`, "c"}},
		{`a\\,b`, []string{`a\\`, "b"}},
		{`"\"hi\"",x`, []string{`"\"hi\""`, "x"}},
		{`'it\'s fine',y`, []string{`'it\'s fine'`, "y"}},
		{"a,", []string{"a", ""}},
		{`" spaced " , unquoted`, []string{`" spaced "`, "unquoted"}},
	}

	for _, test := range testCases {
		got := argSplit(test.input)
		if !reflect.DeepEqual(got, test.expected) {
			t.Errorf("argSplit(%q) = %q, want %q", test.input, got, test.expected)
		}
	}
}

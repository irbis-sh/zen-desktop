package ruletree

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		s      string
		tokens []token
	}{
		{
			"abc123",
			[]token{'a', 'b', 'c', '1', '2', '3'},
		},
		{
			"*",
			[]token{tokenWildcard},
		},
		{
			"a*b",
			[]token{'a', tokenWildcard, 'b'},
		},
		{
			"*a*",
			[]token{tokenWildcard, 'a', tokenWildcard},
		},
		{
			"**",
			[]token{tokenWildcard},
		},
		{
			"a**b",
			[]token{'a', tokenWildcard, 'b'},
		},
		{
			"***a***",
			[]token{tokenWildcard, 'a', tokenWildcard},
		},
		{
			"||",
			[]token{tokenDomainBoundary},
		},
		{
			"|||||",
			[]token{tokenDomainBoundary, tokenDomainBoundary, tokenAnchor},
		},
		{
			"||example.com",
			[]token{tokenDomainBoundary, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'},
		},
		{
			"|",
			[]token{tokenAnchor},
		},
		{
			"example|",
			[]token{'e', 'x', 'a', 'm', 'p', 'l', 'e', tokenAnchor},
		},
		{
			"^",
			[]token{tokenSeparator},
		},
		{
			"a^b",
			[]token{'a', tokenSeparator, 'b'},
		},
		{
			"*||^|",
			[]token{tokenWildcard, tokenDomainBoundary, tokenSeparator, tokenAnchor},
		},
		{
			"a*b||c^d|e",
			[]token{'a', tokenWildcard, 'b', tokenDomainBoundary, 'c', tokenSeparator, 'd', tokenAnchor, 'e'},
		},
		{
			"||example.com/ads/*",
			[]token{tokenDomainBoundary, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', '/', 'a', 'd', 's', '/', tokenWildcard},
		},
	}

	for _, test := range tests {
		if got := tokenize(test.s); !reflect.DeepEqual(got, test.tokens) {
			t.Errorf("Tokenize(%q) = %#v, want %#v", test.s, got, test.tokens)
		}
	}
}

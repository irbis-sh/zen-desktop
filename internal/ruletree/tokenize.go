package ruletree

import "strings"

// token is a single byte of a tokenized pattern: either a literal character
// or one of the special tokens.
type token = byte

// Special tokens are the pattern syntax characters themselves, so a pattern
// is its own tokenized form and node prefixes can share the rule's raw text.
// None of them is ever a literal in a pattern, but a URL can contain them
// (e.g. "?family=A|B"), and such a URL character then selects the edge to a
// special token. That never produces a false match:
//   - "*" leads to the wildcard child, which traversal visits from the same
//     position anyway, so it only adds duplicates, which Get removes.
//   - "^" leads to the separator child, which rejects it: "^" isn't a separator.
//   - "|" leads to the anchor child, which matches only at the end of the URL.
const (
	// tokenWildcard represents "*" and matches any set of characters.
	tokenWildcard token = '*'
	// tokenSeparator represents "^" and matches any character except a letter, digit, or _-.%.
	// It also matches the end of an address.
	tokenSeparator token = '^'
	// tokenAnchor represents "|" and matches the beginning or the end of an address.
	tokenAnchor token = '|'
)

// domainBoundary is "||", which matches domain and subdomain roots. Insert
// strips it from the start of a pattern, and it isn't allowed elsewhere, so it
// has no token.
const domainBoundary = "||"

// tokenize collapses runs of "*", which traversal can't handle. Any other
// pattern is returned as is.
func tokenize(s string) string {
	if !strings.Contains(s, "**") {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '*' && i > 0 && s[i-1] == '*' {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

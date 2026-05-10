package ruletree

type token uint16

const (
	// tokenWildcard represents "*" and matches any set of characters.
	tokenWildcard token = (2 << 7)
	// tokenDomainBoundary represents "||" and matches domain and subdomain roots.
	tokenDomainBoundary token = (2 << 7) + iota
	// tokenSeparator represents "^" and matches any character except a letter, digit, or _-.%.
	// It also matches the end of an address.
	tokenSeparator token = (2 << 7) + iota
	// tokenAnchor represents "|" and matches the beginning or the end of an address.
	tokenAnchor token = (2 << 7) + iota
)

func tokenize(s string) []token {
	var tokens []token
	for i := 0; i < len(s); {
		switch s[i] {
		case '*':
			if len(tokens) == 0 || tokens[len(tokens)-1] != tokenWildcard {
				tokens = append(tokens, tokenWildcard)
			}
		case '|':
			switch {
			case i+1 < len(s) && s[i+1] == '|':
				tokens = append(tokens, tokenDomainBoundary)
				i++
			default:
				tokens = append(tokens, tokenAnchor)
			}
		case '^':
			tokens = append(tokens, tokenSeparator)
		default:
			tokens = append(tokens, token(s[i]))
		}
		i++
	}
	return tokens
}

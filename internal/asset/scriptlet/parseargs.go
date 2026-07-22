package scriptlet

import (
	"errors"
	"fmt"
)

// This file decodes scriptlet rule argument lists into their intended string
// values. Filter-list syntax has its own minimal escaping, distinct from JS
// string literal escaping:
//
//   - Canonical (AdGuard) syntax quotes every argument with ' or " (one style
//     per rule); a backslash escapes the bounding quote character.
//   - uBlock Origin syntax separates unquoted arguments with commas; a
//     backslash-escaped comma (\,) is a literal comma. An argument may also be
//     wrapped in single quotes, double quotes, or backticks, which makes
//     commas inside it literal and backslash escape the bounding quote.
//
// In both syntaxes, every other backslash is a literal byte: regex arguments
// like '/\d+\.\d+/' depend on this. Reference implementations:
//   - AdgScriptletInjectionBodyParser in AdGuard's agtree:
//     https://github.com/AdguardTeam/tsurlfilter/blob/34b7e34052ee9e5cd901d5c752bdffebae9b1325/packages/agtree/src/parser/cosmetic/scriptlet-body/adg-scriptlet-injection-body-parser.ts
//   - ArglistParser in uBlock Origin:
//     https://github.com/gorhill/uBlock/blob/7dfeb93a1bebcb5e3b406496ea96a3f68d46dfc5/src/js/arglist-parser.js

// parseCanonicalArgList decodes the argument list of a canonical-syntax rule,
// e.g. 'abort-on-property-read', 'document.createElement'.
func parseCanonicalArgList(s string) ([]string, error) {
	var args []string
	var quote byte // Quote style of the whole list, detected from the first argument.

	i := skipWhitespace(s, 0)
	for i < len(s) {
		if len(args) > 0 {
			if s[i] != ',' {
				return nil, fmt.Errorf("expected comma at index %d", i)
			}
			i = skipWhitespace(s, i+1)
		}
		if i == len(s) || (s[i] != '\'' && s[i] != '"') {
			return nil, fmt.Errorf("expected quote at index %d", i)
		}
		if quote == 0 {
			quote = s[i]
		} else if s[i] != quote {
			return nil, errors.New("inconsistent quote types")
		}
		closing := findUnescaped(s, quote, i+1)
		if closing == -1 {
			return nil, fmt.Errorf("unclosed argument at index %d", i)
		}
		args = append(args, unescapeChar(s[i+1:closing], quote))
		i = skipWhitespace(s, closing+1)
	}

	if len(args) == 0 {
		return nil, errors.New("empty argument list")
	}
	return args, nil
}

// parseUboArgList decodes the argument list of a uBlock Origin-syntax rule,
// e.g. no-xhr-if, /atr\?.+?&rt=\d+/ method:POST.
func parseUboArgList(s string) ([]string, error) {
	var args []string

	i := 0
	for i < len(s) {
		i = skipWhitespace(s, i)
		if i == len(s) {
			// Only whitespace remains after the last consumed separator: uBO
			// emits one final empty argument here, so "a, " decodes to
			// ["a", ""] while "a," decodes to just ["a"].
			if len(args) > 0 {
				args = append(args, "")
			}
			break
		}
		start := i

		if c := s[i]; c == '\'' || c == '"' || c == '`' {
			// A quoted argument is only recognized when its closing quote is
			// followed by a separator or the end of the list; otherwise the
			// quote is an ordinary character (matching uBO's ArglistParser).
			if closing := findUnescaped(s, c, i+1); closing != -1 {
				if next := skipWhitespace(s, closing+1); next == len(s) || s[next] == ',' {
					args = append(args, unescapeChar(s[i+1:closing], c))
					i = min(next+1, len(s))
					continue
				}
			}
		}

		end := findUnescaped(s, ',', start)
		if end == -1 {
			i = len(s)
			end = len(s)
		} else {
			i = end + 1
		}
		// Trim trailing whitespace; leading whitespace was skipped above.
		for end > start && isWhitespace(s[end-1]) {
			end--
		}
		args = append(args, unescapeChar(s[start:end], ','))
	}

	if len(args) == 0 {
		return nil, errors.New("empty argument list")
	}
	return args, nil
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t'
}

// skipWhitespace returns the index of the first non-whitespace character in s at or
// after index i, or len(s) if there is none.
func skipWhitespace(s string, i int) int {
	for i < len(s) && isWhitespace(s[i]) {
		i++
	}
	return i
}

// findUnescaped returns the index of the first unescaped occurrence of c in s
// at or after index from, or -1 if there is none. An occurrence is escaped
// when preceded by an odd-length run of backslashes.
func findUnescaped(s string, c byte, from int) int {
	run := 0 // Length of the backslash run preceding the current character.
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '\\':
			run++
		case c:
			if run%2 == 0 {
				return i
			}
			run = 0
		default:
			run = 0
		}
	}
	return -1
}

// unescapeChar removes the backslash escaping each escaped occurrence of c,
// leaving all other backslashes intact. An occurrence is escaped when preceded
// by an odd-length run of backslashes.
func unescapeChar(s string, c byte) string {
	out := make([]byte, 0, len(s))
	run := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' {
			run++
			out = append(out, ch)
			continue
		}
		if ch == c && run%2 == 1 {
			out = out[:len(out)-1]
		}
		run = 0
		out = append(out, ch)
	}
	return string(out)
}

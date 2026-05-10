package cosmetic

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var unicodeEscapeRegex = regexp.MustCompile(`\\[0-9A-Fa-f]{1,6}`)

// sanitizeCSSSelector validates and sanitizes a CSS selector.
func sanitizeCSSSelector(selectorInput string) (string, error) {
	if strings.Contains(selectorInput, "</style>") {
		return "", errors.New("selector contains '</style>' which is not allowed")
	}

	selector := decodeUnicodeEscapes(selectorInput)
	if !hasBalancedQuotesAndBrackets(selector) {
		return "", errors.New("selector has unbalanced quotes or brackets")
	}

	if err := validateSelector(selector); err != nil {
		return "", fmt.Errorf("sanitize selector: %w", err)
	}

	return selector, nil
}

// decodeUnicodeEscapes replaces CSS Unicode escapes with their actual characters.
func decodeUnicodeEscapes(s string) string {
	return unicodeEscapeRegex.ReplaceAllStringFunc(s, func(match string) string {
		hexDigits := match[1:] // Skip the leading backslash
		r, err := strconv.ParseInt(hexDigits, 16, 32)
		if err != nil {
			return match
		}
		return string(rune(r))
	})
}

// hasBalancedQuotesAndBrackets checks for balanced quotes and brackets in the selector.
func hasBalancedQuotesAndBrackets(s string) bool {
	var stack []rune
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for _, c := range s {
		if escaped {
			escaped = false
			continue
		}

		if c == '\\' {
			escaped = true
			continue
		}

		if inSingleQuote {
			if c == '\'' {
				inSingleQuote = false
			}
			continue
		}

		if inDoubleQuote {
			if c == '"' {
				inDoubleQuote = false
			}
			continue
		}

		if c == '\'' {
			inSingleQuote = true
			continue
		}

		if c == '"' {
			inDoubleQuote = true
			continue
		}

		switch c {
		case '(', '[', '{':
			stack = append(stack, c)
		case ')', ']', '}':
			if len(stack) == 0 {
				return false
			}
			last := stack[len(stack)-1]
			if (c == ')' && last != '(') ||
				(c == ']' && last != '[') ||
				(c == '}' && last != '{') {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}

	return !inSingleQuote && !inDoubleQuote && len(stack) == 0 && !escaped
}

// validateSelector checks for dangerous sequences in the selector.
func validateSelector(s string) error {
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	runes := []rune(s)

	for i := 0; i < len(runes); i++ {
		c := runes[i]

		if escaped {
			escaped = false
			continue
		}

		if c == '\\' {
			escaped = true
			continue
		}

		if inSingleQuote {
			if c == '\'' {
				inSingleQuote = false
			}
			continue
		}

		if inDoubleQuote {
			if c == '"' {
				inDoubleQuote = false
			}
			continue
		}

		if c == '\'' {
			inSingleQuote = true
			continue
		}

		if c == '"' {
			inDoubleQuote = true
			continue
		}

		if !inSingleQuote && !inDoubleQuote {
			// Check for dangerous sequences.
			if c == '/' && i+1 < len(runes) && runes[i+1] == '*' {
				return errors.New("found '/*' outside of quotes")
			}

			if c == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				return errors.New("found '*/' outside of quotes")
			}

			if c == '{' || c == '}' || c == ';' || c == '@' {
				return fmt.Errorf("found dangerous character '%c' outside of quotes", c)
			}
		}
	}

	return nil
}

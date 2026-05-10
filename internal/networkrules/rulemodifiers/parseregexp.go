package rulemodifiers

import (
	"fmt"
	"regexp"
)

var (
	// regexpRegexp is a regexp that matches a regexp.
	regexpRegexp = regexp.MustCompile(`^/(.+)/i?$`)
)

// parseRegexp parses regulars expressions contained in rule modifiers.
// It returns nil as the first value if the input doesn't look like a regular expression.
func parseRegexp(s string) (*regexp.Regexp, error) {
	match := regexpRegexp.FindStringSubmatch(s)
	if match == nil {
		return nil, nil
	}

	regexpBody := match[1]
	if s[len(s)-1] == 'i' {
		// Filter lists are designed for JS-based applications whose regex flag syntax is incompatible with that of Go.
		// Therefore, we explicitly convert one to another to maintain compatibility.
		regexpBody = "(?i)" + regexpBody
	}

	regexp, err := regexp.Compile(regexpBody)
	if err != nil {
		return nil, fmt.Errorf("compile regexp: %w", err)
	}

	return regexp, nil
}

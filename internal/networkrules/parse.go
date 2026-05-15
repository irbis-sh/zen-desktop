package networkrules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/exceptionrule"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
)

var (
	reHosts       = regexp.MustCompile(`^(?:0\.0\.0\.0|127\.0\.0\.1)\s(.+)`)
	reHostsIgnore = regexp.MustCompile(`^(?:0\.0\.0\.0|broadcasthost|local|localhost(?:\.localdomain)?|ip6-\w+)$`)
)

func (nr *NetworkRules) ParseRule(rawRule string, filterName *string) (isException bool, err error) {
	if matches := reHosts.FindStringSubmatch(rawRule); matches != nil {
		hostsField := matches[1]
		if commentIndex := strings.IndexByte(hostsField, '#'); commentIndex != -1 {
			hostsField = hostsField[:commentIndex]
		}

		// An IP address may be followed by multiple hostnames.
		//
		// As stated in https://man.freebsd.org/cgi/man.cgi?hosts(5):
		// "Items are separated by any number of blanks and/or tab characters."
		hosts := strings.Fields(hostsField)

		for _, host := range hosts {
			if reHostsIgnore.MatchString(host) {
				continue
			}

			pattern := fmt.Sprintf("||%s^", host)
			if err := nr.primaryStore.Insert(pattern, &rule.Rule{
				RawRule:    rawRule,
				FilterName: filterName,
				Document:   true,
			}); err != nil {
				return false, fmt.Errorf("insert hosts rule: %w", err)
			}
		}

		return false, nil
	}

	if strings.HasPrefix(rawRule, "@@") {
		r := &exceptionrule.ExceptionRule{
			RawRule:    rawRule,
			FilterName: filterName,
		}

		pattern, modifiers := parseRuleParts(rawRule[2:])
		if modifiers != nil {
			if err := r.ParseModifiers(modifiers); err != nil {
				return false, fmt.Errorf("parse modifiers: %v", err)
			}
		}
		if err := nr.exceptionStore.Insert(pattern, r); err != nil {
			return false, fmt.Errorf("insert exception rule: %w", err)
		}

		return true, nil
	}

	r := &rule.Rule{
		RawRule:    rawRule,
		FilterName: filterName,
	}

	pattern, modifiers := parseRuleParts(rawRule)
	if modifiers != nil {
		if err := r.ParseModifiers(modifiers); err != nil {
			return false, fmt.Errorf("parse modifiers: %v", err)
		}
	}
	if err := nr.primaryStore.Insert(pattern, r); err != nil {
		return false, fmt.Errorf("insert rule: %w", err)
	}

	return false, nil
}

// parseRuleParts splits rawRule into its pattern and modifier list.
func parseRuleParts(rawRule string) (pattern string, modifiers []string) {
	if pattern, modifiers, ok := parseRegexpRuleParts(rawRule); ok {
		return pattern, modifiers
	}

	pattern, rawModifiers, found := strings.Cut(rawRule, "$")
	if found {
		modifiers = splitModifiers(rawModifiers)
	}
	return pattern, modifiers
}

// parseRegexpRuleParts splits a slash-delimited regexp rule into its pattern and modifier list.
// Reports ok only if the rule looks like a regexp rule.
func parseRegexpRuleParts(rawRule string) (pattern string, modifiers []string, ok bool) {
	if len(rawRule) < 2 || rawRule[0] != '/' {
		return "", nil, false
	}

	end := regexpPatternEnd(rawRule)
	if end == -1 {
		return "", nil, false
	}

	pattern = rawRule[:end+1]
	if end+1 < len(rawRule) {
		modifiers = splitModifiers(rawRule[end+2:])
	}
	return pattern, modifiers, true
}

func regexpPatternEnd(s string) int {
	escaped := false
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			escaped = !escaped
		case '/':
			if escaped {
				escaped = false
				continue
			}
			if i+1 == len(s) || s[i+1] == '$' { // End-of-string or modifier delimiter.
				return i
			}
		default:
			escaped = false
		}
	}
	return -1
}

// splitModifiers splits by unescaped commas.
// Empty fields are preserved (like strings.Split).
func splitModifiers(s string) []string {
	var res []string
	var b strings.Builder
	escaped := false

	for _, r := range s {
		switch r {
		case '\\':
			if escaped {
				b.WriteRune('\\')
			}
			escaped = !escaped
		case ',':
			if escaped {
				b.WriteRune(',')
				escaped = false
			} else {
				res = append(res, b.String())
				b.Reset()
			}
		default:
			if escaped {
				b.WriteRune('\\')
				escaped = false
			}
			b.WriteRune(r)
		}
	}

	if escaped {
		b.WriteRune('\\')
	}
	res = append(res, b.String())
	return res
}

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

			// Derive the pattern from the raw rule so the rule tree's domain
			// key shares the raw rule's memory instead of keeping a copy.
			const modifiers = "$all"
			hostsRule := "||" + host + "^" + modifiers
			pattern := hostsRule[:len(hostsRule)-len(modifiers)]
			r := &rule.Rule{
				RawRule:    hostsRule,
				FilterName: filterName,
			}
			if err := nr.interner.Parse(r, modifiers[1:]); err != nil {
				return false, fmt.Errorf("parse hosts rule modifiers: %v", err)
			}
			if err := nr.primaryStore.Insert(pattern, r); err != nil {
				return false, fmt.Errorf("insert hosts rule: %w", err)
			}
		}

		return false, nil
	}

	if strings.HasPrefix(rawRule, "@@") {
		r := &exceptionrule.ExceptionRule{
			Rule: rule.Rule{
				RawRule:    rawRule,
				FilterName: filterName,
			},
		}

		pattern, rawModifiers, hasModifiers := parseRuleParts(rawRule[2:])
		if hasModifiers {
			if err := nr.interner.Parse(&r.Rule, rawModifiers); err != nil {
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

	pattern, rawModifiers, hasModifiers := parseRuleParts(rawRule)
	if hasModifiers {
		if err := nr.interner.Parse(r, rawModifiers); err != nil {
			return false, fmt.Errorf("parse modifiers: %v", err)
		}
	}
	if err := nr.primaryStore.Insert(pattern, r); err != nil {
		return false, fmt.Errorf("insert rule: %w", err)
	}

	return false, nil
}

// parseRuleParts splits rawRule into its pattern and unsplit modifier text.
func parseRuleParts(rawRule string) (pattern, rawModifiers string, hasModifiers bool) {
	// A regexp pattern may contain '$', so cut only after its closing slash,
	// which regexpPatternEnd guarantees is followed by '$' or nothing.
	if strings.HasPrefix(rawRule, "/") {
		if end := regexpPatternEnd(rawRule); end != -1 {
			_, rawModifiers, hasModifiers = strings.Cut(rawRule[end+1:], "$")
			return rawRule[:end+1], rawModifiers, hasModifiers
		}
	}
	return strings.Cut(rawRule, "$")
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

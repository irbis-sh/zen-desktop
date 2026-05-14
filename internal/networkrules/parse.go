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
			nr.primaryStore.Insert(pattern, &rule.Rule{
				RawRule:    rawRule,
				FilterName: filterName,
				Document:   true,
			})
		}

		return false, nil
	}

	if strings.HasPrefix(rawRule, "@@") {
		r := &exceptionrule.ExceptionRule{
			RawRule:    rawRule,
			FilterName: filterName,
		}

		pattern, modifiers, isRegexp := parseRuleParts(rawRule[2:])
		if isRegexp {
			// This is a regexp rule.
			// TODO: implement proper support for regexp rules.
			return true, nil
		}
		if modifiers != nil {
			if err := r.ParseModifiers(modifiers); err != nil {
				return false, fmt.Errorf("parse modifiers: %v", err)
			}
		}
		nr.exceptionStore.Insert(pattern, r)

		return true, nil
	}

	r := &rule.Rule{
		RawRule:    rawRule,
		FilterName: filterName,
	}

	pattern, modifiers, isRegexp := parseRuleParts(rawRule)
	if isRegexp {
		// This is a regexp rule.
		// TODO: implement proper support for regexp rules.
		return false, nil
	}
	if modifiers != nil {
		if err := r.ParseModifiers(modifiers); err != nil {
			return false, fmt.Errorf("parse modifiers: %v", err)
		}
	}
	nr.primaryStore.Insert(pattern, r)

	return false, nil
}

func parseRuleParts(rawRule string) (pattern string, modifiers []string, isRegexp bool) {
	pattern, rawModifiers, found := strings.Cut(rawRule, "$")
	isRegexp = pattern != "" && pattern[0] == '/' && pattern[len(pattern)-1] == '/'
	if found {
		modifiers = splitModifiers(rawModifiers)
	}

	return pattern, modifiers, isRegexp
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

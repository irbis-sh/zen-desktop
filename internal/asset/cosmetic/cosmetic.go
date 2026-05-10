package cosmetic

import (
	"regexp"

	"github.com/irbis-sh/zen-desktop/internal/asset/extendedcss"
)

var (
	ruleRegex = regexp.MustCompile(`^.*#@?#(.+)$`)
)

// IsRule returns true if the given string is a cosmetic rule.
func IsRule(s string) bool {
	match := ruleRegex.FindStringSubmatch(s)
	if match == nil {
		return false
	}

	body := match[1]
	// uBlock Origin uses the same syntax for cosmetic and extended CSS rules,
	// so we return false if the rule body contains any extended pseudo-classes.
	return !extendedcss.ExtPseudoClassRegex.MatchString(body)
}

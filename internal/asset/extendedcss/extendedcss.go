package extendedcss

import "regexp"

var (
	// ExtPseudoClassRegex matches extended pseudo-classes.
	ExtPseudoClassRegex = regexp.MustCompile(`:(?:has-text|contains|matches-attr|matches-css(?:-before|-after)?|matches-media|matches-path|matches-prop(?:erty)?|min-text-length|others|upward|xpath|nth-ancestor|watch-attr|remove|style|-abp-(?:contains|has))`)

	ruleRegex = regexp.MustCompile(`^.+#@?\??#.+$`)
)

// IsRule returns true if the given string is an extended CSS rule.
func IsRule(s string) bool {
	return ruleRegex.MatchString(s)
}

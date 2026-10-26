package exceptionrule

import "github.com/irbis-sh/zen-desktop/internal/networkrules/rule"

type ExceptionRule struct {
	rule.Rule
}

// Cancels reports whether er cancels r. The caller has already matched both rules against
// the request or response, so their content types and condition modifiers take no part
// here: they decide whether the exception applies, not which rules it cancels. A broad
// block rule such as "||ads.example^" is therefore cancelled by
// "@@||ads.example^$script,domain=a.com".
func (er *ExceptionRule) Cancels(r *rule.Rule) bool {
	if r.Important() && !er.Important() {
		return false
	}

	for _, exc := range er.ActionModifiers() {
		found := false
		for _, basic := range r.ActionModifiers() {
			if exc.Cancels(basic) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	for _, exc := range er.QueryModifiers() {
		found := false
		for _, basic := range r.QueryModifiers() {
			if exc.Cancels(basic) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

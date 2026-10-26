package exceptionrule

import (
	"net/http"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/contenttype"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
)

type ExceptionRule struct {
	rule.Rule
}

// Cancels reports whether er cancels r. The caller has already matched both rules against
// the request or response, so their condition modifiers take no part here: they decide
// whether the exception applies, not which rules it cancels. A broad block rule such as
// "||ads.example^" is therefore cancelled by "@@||ads.example^$script,domain=a.com".
func (er *ExceptionRule) Cancels(r *rule.Rule) bool {
	if r.Important() && !er.Important() {
		return false
	}

	if er.Document() && !r.Document() && !r.All() {
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

// ShouldMatchReq reports whether the rule matches a request of type t.
func (er *ExceptionRule) ShouldMatchReq(req *http.Request, t contenttype.Type) bool {
	return er.ModifiersMatchReq(req, t)
}

// ShouldMatchRes reports whether the rule matches a response of type t.
func (er *ExceptionRule) ShouldMatchRes(res *http.Response, t contenttype.Type) bool {
	return er.ModifiersMatchRes(res, t)
}

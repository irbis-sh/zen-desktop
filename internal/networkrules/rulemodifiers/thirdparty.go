package rulemodifiers

import (
	"net/http"
)

// https://adguard.com/kb/general/ad-filtering/create-own-filters/#third-party-modifier
type ThirdPartyModifier struct {
	inverted bool
}

var _ ConditionModifier = (*ThirdPartyModifier)(nil)

func (m *ThirdPartyModifier) Parse(modifier string) error {
	if modifier[0] == '~' {
		m.inverted = true
	}
	return nil
}

func (m *ThirdPartyModifier) ShouldMatchReq(req Request) bool {
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Sec-Fetch-Site
	switch req.Header.Get("Sec-Fetch-Site") {
	case "cross-site":
		return !m.inverted
	case "same-origin", "same-site":
		return m.inverted
	default:
		return false
	}
}

// ShouldMatchRes reports whether m matches res.
func (m *ThirdPartyModifier) ShouldMatchRes(res *http.Response) bool {
	if res.Request == nil {
		return false
	}
	// Reuse the request-side logic so response matching stays in sync with
	// request matching.
	return m.ShouldMatchReq(NewRequest(res.Request))
}

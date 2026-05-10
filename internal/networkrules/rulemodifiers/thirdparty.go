package rulemodifiers

import (
	"net/http"
)

// https://adguard.com/kb/general/ad-filtering/create-own-filters/#third-party-modifier
type ThirdPartyModifier struct {
	inverted bool
}

var _ MatchingModifier = (*ThirdPartyModifier)(nil)

func (m *ThirdPartyModifier) Parse(modifier string) error {
	if modifier[0] == '~' {
		m.inverted = true
	}
	return nil
}

func (m *ThirdPartyModifier) ShouldMatchReq(req *http.Request) bool {
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

func (m *ThirdPartyModifier) ShouldMatchRes(_ *http.Response) bool {
	return false
}

func (m *ThirdPartyModifier) Cancels(modifier Modifier) bool {
	other, ok := modifier.(*ThirdPartyModifier)
	if !ok {
		return false
	}

	return m.inverted == other.inverted
}

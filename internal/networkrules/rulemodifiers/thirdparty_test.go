package rulemodifiers

import (
	"net/http"
	"net/url"
	"testing"
)

func TestThirdPartyModifierShouldMatchRes(t *testing.T) {
	t.Parallel()

	m := ThirdPartyModifier{}
	if err := m.Parse("third-party"); err != nil {
		t.Fatal(err)
	}

	res := &http.Response{Request: &http.Request{Header: http.Header{}, URL: &url.URL{Scheme: "https", Host: "example.com"}}}
	res.Request.Header.Set("Sec-Fetch-Site", "cross-site")
	if !m.ShouldMatchRes(res) {
		t.Error("ThirdPartyModifier.ShouldMatchRes(cross-site) = false, want true")
	}
	res.Request.Header.Set("Sec-Fetch-Site", "same-origin")
	if m.ShouldMatchRes(res) {
		t.Error("ThirdPartyModifier.ShouldMatchRes(same-origin) = true, want false")
	}

	inv := ThirdPartyModifier{}
	if err := inv.Parse("~third-party"); err != nil {
		t.Fatal(err)
	}
	res.Request.Header.Set("Sec-Fetch-Site", "cross-site")
	if inv.ShouldMatchRes(res) {
		t.Error("inverted ThirdPartyModifier.ShouldMatchRes(cross-site) = true, want false")
	}
	res.Request.Header.Set("Sec-Fetch-Site", "same-origin")
	if !inv.ShouldMatchRes(res) {
		t.Error("inverted ThirdPartyModifier.ShouldMatchRes(same-origin) = false, want true")
	}

}

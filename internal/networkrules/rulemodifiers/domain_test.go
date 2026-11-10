package rulemodifiers

import (
	"net/http"
	"net/url"
	"testing"
)

func TestDomainModifier(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name        string
		modifier    string
		referer     string
		shouldMatch bool
	}{
		{"Single domain - match", "domain=example.com", "http://example.com/", true},
		{"Single domain - subdomain match", "domain=example.com", "http://sub.example.com/path", true},
		{"Single domain - no match with unrelated domain ending with domain", "domain=example.com", "http://testexample.com/", false},
		{"Single domain - no match with unrelated domain", "domain=example.com", "http://example.org/", false},
		{"Single inverted domain - match", "domain=~example.com", "http://test.com/", true},
		{"Single inverted domain - no match", "domain=~example.com", "http://example.com/", false},
		{"TLD - no match with unrelated subdomain", "domain=google.*", "http://google.schmoogle.com/", false},
		{"TLD - match", "domain=example.*", "http://example.com/", true},
		{"TLD - subdomain and path match", "domain=example.*", "https://www.example.co.uk/some/path", true},
		{"TLD - no match with unrelated domain ending with tld", "domain=example.*", "https://testexample.com", false},
		{"TLD - no match with subdomain", "domain=example.*", "http://example.test1.test2.com", false},
		{"TLD - match with co.uk suffix", "domain=example.*", "http://example.co.uk/", true},
		{"TLD - match with com.au suffix", "domain=example.*", "http://example.com.au/", true},
		{"TLD - no match with subdomain of co.uk", "domain=example.*", "http://example.fake.co.uk/", false},
		{"TLD - no match with unrelated domain", "domain=example.*", "http://test.com", false},
		{"Regex - match com", `domain=/^example\.(com|org)$/`, "http://example.com/", true},
		{"Regex - match org", `domain=/^example\.(com|org)$/`, "http://example.org/", true},
		{"Regex - no match", `domain=/^example\.(com|org)$/`, "http://example.net/", false},
		{"Multiple domains - match com", "domain=example.com|example.org", "http://example.com/", true},
		{"Multiple domains - match org", "domain=example.com|example.org", "http://example.org/", true},
		{"Multiple domains - no match", "domain=example.com|example.org", "http://example.net/", false},
		{"Multiple inverted domains - match", "domain=~example.com|~example.org", "http://example.net/", true},
		{"Multiple inverted domains - no match com", "domain=~example.com|~example.org", "http://example.com/", false},
		{"Multiple inverted domains - no match org", "domain=~example.com|~example.org", "http://example.org/", false},
		{"Unparsable Referer - no match", "domain=example.com", "https://example.com:abc/", false},
		{"Unparsable Referer - no inverted match", "domain=~example.com", "https://example.com:abc/", false},
		{"Referer without scheme - no match", "domain=example.com", "example.com/page", false},
		{"Referer without scheme - inverted match", "domain=~example.com", "example.com/page", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newDomainModifier(t, tt.modifier)
			req := newRequestWithReferer(tt.referer)
			if got := m.ShouldMatchReq(NewRequest(req)); got != tt.shouldMatch {
				t.Errorf("domainModifier{%s}.ShouldMatchReq(%s) = %v, want %v", tt.modifier, tt.referer, got, tt.shouldMatch)
			}
		})
	}

	t.Run("Should match inverted domains against request without Referer", func(t *testing.T) {
		t.Parallel()

		modifier := "domain=~example.com|~example.org"
		m := newDomainModifier(t, modifier)
		reqWithoutReferer := &http.Request{
			URL: &url.URL{
				Scheme: "https",
				Host:   "somethingelse.com",
			},
		}

		want := true
		if got := m.ShouldMatchReq(NewRequest(reqWithoutReferer)); got != want {
			t.Errorf("domainModifier{%s}.ShouldMatchReq() = %v, want %v", modifier, got, want)
		}
	})

	t.Run("Should fail on empty modifier", func(t *testing.T) {
		t.Parallel()
		m := DomainModifier{}
		if err := m.Parse("domain="); err == nil {
			t.Error("domainModifier.Parse(\"domain=\") = nil, want error")
		}
	})

	t.Run("Should fail on inverted and non-inverted domains", func(t *testing.T) {
		t.Parallel()
		m := DomainModifier{}
		if err := m.Parse("domain=example.com|~example.org"); err == nil {
			t.Error("domainModifier.Parse(\"domain=example.com|~example.org\") = nil, want error")
		}
	})

	t.Run("Should not match inverted domains when request URL matches and no Referer is present", func(t *testing.T) {
		t.Parallel()

		modifier := "domain=~google.*"
		m := newDomainModifier(t, modifier)

		req := &http.Request{
			URL: &url.URL{
				Scheme: "https",
				Host:   "www.google.com",
			},
		}

		want := false
		got := m.ShouldMatchReq(NewRequest(req))

		if got != want {
			t.Errorf("domainModifier{%q}.ShouldMatchReq() = %v, want %v", modifier, got, want)
		}
	})
}

func newDomainModifier(t *testing.T, domain string) DomainModifier {
	t.Helper()
	m := DomainModifier{}
	if err := m.Parse(domain); err != nil {
		t.Fatal(err)
	}
	return m
}

func newRequestWithReferer(referer string) *http.Request {
	return &http.Request{
		URL: &url.URL{Scheme: "https", Host: "cdn.example.net"},
		Header: http.Header{
			"Referer": []string{referer},
		},
	}
}

func TestDomainModifierShouldMatchRes(t *testing.T) {
	t.Parallel()

	m := newDomainModifier(t, "domain=example.com")
	res := &http.Response{
		Request: &http.Request{
			URL: &url.URL{Scheme: "https", Host: "example.com"},
		},
	}
	if !m.ShouldMatchRes(res) {
		t.Error("domainModifier.ShouldMatchRes(res) = false, want true")
	}

	res.Request.URL.Host = "other.com"
	if m.ShouldMatchRes(res) {
		t.Error("domainModifier.ShouldMatchRes(res) = true, want false")
	}

	// inverted
	inv := newDomainModifier(t, "domain=~example.com")
	res.Request.URL.Host = "example.com"
	if inv.ShouldMatchRes(res) {
		t.Error("inverted domainModifier.ShouldMatchRes(res) = true, want false")
	}
	res.Request.URL.Host = "other.com"
	if !inv.ShouldMatchRes(res) {
		t.Error("inverted domainModifier.ShouldMatchRes(res) = false, want true")
	}

}

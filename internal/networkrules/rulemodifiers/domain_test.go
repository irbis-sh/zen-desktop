package rulemodifiers

import (
	"net/http"
	"net/url"
	"regexp"
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
		{"TLD - match", "domain=example.*", "http://example.com/", true},
		{"TLD - subdomain and path match", "domain=example.*", "https://www.example.co.uk/some/path", true},
		{"TLD - no match with unrelated domain ending with tld", "domain=example.*", "https://testexample.com", false},
		{"TLD - no match with subdomain", "domain=example.*", "http://example.test1.test2.com", false},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newDomainModifier(t, tt.modifier)
			req := newRequestWithReferer(tt.referer)
			if got := m.ShouldMatchReq(req); got != tt.shouldMatch {
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
		if got := m.ShouldMatchReq(reqWithoutReferer); got != want {
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
		got := m.ShouldMatchReq(req)

		if got != want {
			t.Errorf("domainModifier{%q}.ShouldMatchReq() = %v, want %v", modifier, got, want)
		}
	})

	t.Run("Cancels", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			a    DomainModifier
			b    DomainModifier
			want bool
		}{
			{
				"Should cancel - empty modifiers",
				DomainModifier{},
				DomainModifier{},
				true,
			},
			{
				"Should cancel - different order of entries",
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
						{regular: "reg2", tld: "top2", regexp: regexp.MustCompile("2")},
					},
					inverted: true,
				},
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg2", tld: "top2", regexp: regexp.MustCompile("2")},
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
					},
					inverted: true,
				},
				true,
			},
			{
				"Should cancel - regex is nil",
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: nil},
						{regular: "reg2", tld: "top2", regexp: nil},
					},
					inverted: true,
				},
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg2", tld: "top2", regexp: nil},
						{regular: "reg1", tld: "top1", regexp: nil},
					},
					inverted: true,
				},
				true,
			},
			{
				"Should not cancel - Different regular values",
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
					},
					inverted: true,
				},
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg2", tld: "top1", regexp: regexp.MustCompile("1")},
					},
					inverted: true,
				},
				false,
			},
			{
				"Should not cancel - Different TLD values",
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
					},
					inverted: true,
				},
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top2", regexp: regexp.MustCompile("1")},
					},
					inverted: true,
				},
				false,
			},
			{
				"Should not cancel - Different regex patterns",
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
					},
					inverted: true,
				},
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("2")},
					},
					inverted: true,
				},
				false,
			},
			{
				"Should not cancel - Different inverted value",
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
					},
					inverted: false,
				},
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
					},
					inverted: true,
				},
				false,
			},
			{
				"Should not cancel - One of regexes is nil",
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg1", tld: "top1", regexp: regexp.MustCompile("1")},
						{regular: "reg2", tld: "top2", regexp: regexp.MustCompile("2")},
					},
					inverted: true,
				},
				DomainModifier{
					entries: []domainModifierEntry{
						{regular: "reg2", tld: "top2", regexp: nil},
						{regular: "reg1", tld: "top1", regexp: nil},
					},
					inverted: true,
				},
				false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := tt.a.Cancels(&tt.b)
				if got != tt.want {
					t.Errorf("domainModifier.Cancels() = %t, want %t", got, tt.want)
				}
			})
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
		Header: http.Header{
			"Referer": []string{referer},
		},
	}
}

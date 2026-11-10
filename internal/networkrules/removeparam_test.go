package networkrules

import (
	"net/http"
	"slices"
	"testing"
)

// TestPatternlessRemoveParam covers $removeparam rules without a URL pattern,
// such as "$removeparam=fbclid". They don't match a request whose query they
// can't change, but exceptions to them do.
func TestPatternlessRemoveParam(t *testing.T) {
	t.Parallel()

	documentHeaders := http.Header{"Sec-Fetch-Dest": []string{"document"}}
	scriptHeaders := http.Header{"Sec-Fetch-Dest": []string{"script"}}

	tests := []struct {
		name    string
		rules   []string
		url     string
		headers http.Header
		// wantURL is the redirect URL, or "" if the request isn't changed.
		wantURL string
		// wantApplied is the raw text of the applied rules, in order.
		wantApplied []string
	}{
		{
			name:        "removes the parameter",
			rules:       []string{`$removeparam=utm_source`},
			url:         "https://example.com/?utm_source=x&a=1",
			wantURL:     "https://example.com/?a=1",
			wantApplied: []string{`$removeparam=utm_source`},
		},
		{
			name:        "star pattern removes the parameter",
			rules:       []string{`*$removeparam=utm_source`},
			url:         "https://example.com/?utm_source=x&a=1",
			wantURL:     "https://example.com/?a=1",
			wantApplied: []string{`*$removeparam=utm_source`},
		},
		{
			name:  "no query",
			rules: []string{`$removeparam=utm_source`},
			url:   "https://example.com/",
		},
		{
			name:  "parameter absent",
			rules: []string{`$removeparam=utm_source`},
			url:   "https://example.com/?a=1",
		},
		{
			name:  "query of only separators",
			rules: []string{`$removeparam=utm_source`, `$removeparam=/^utm_/`, `$removeparam`, `$removeparam=~id`},
			url:   "https://example.com/?&&",
		},
		{
			name:  "name is case sensitive",
			rules: []string{`$removeparam=utm_source`},
			url:   "https://example.com/?UTM_SOURCE=x",
		},
		{
			name:        "name is compared after decoding",
			rules:       []string{`$removeparam=utm_source`},
			url:         "https://example.com/?utm%5Fsource=x&a=1",
			wantURL:     "https://example.com/?a=1",
			wantApplied: []string{`$removeparam=utm_source`},
		},
		{
			name:        "removes every value of a repeated parameter",
			rules:       []string{`$removeparam=utm_source`},
			url:         "https://example.com/?utm_source=x&utm_source=y&a=1",
			wantURL:     "https://example.com/?a=1",
			wantApplied: []string{`$removeparam=utm_source`},
		},
		{
			name:        "removes a parameter without a value",
			rules:       []string{`$removeparam=utm_source`},
			url:         "https://example.com/?utm_source&a=1",
			wantURL:     "https://example.com/?a=1",
			wantApplied: []string{`$removeparam=utm_source`},
		},
		{
			name:        "domain met",
			rules:       []string{`$removeparam=a,domain=site.com`},
			url:         "https://example.com/?a=1&b=2",
			headers:     http.Header{"Referer": []string{"https://site.com/"}},
			wantURL:     "https://example.com/?b=2",
			wantApplied: []string{`$removeparam=a,domain=site.com`},
		},
		{
			name:    "domain not met",
			rules:   []string{`$removeparam=a,domain=site.com`},
			url:     "https://example.com/?a=1&b=2",
			headers: http.Header{"Referer": []string{"https://other.com/"}},
		},
		{
			name:        "content type met",
			rules:       []string{`$doc,removeparam=a`},
			url:         "https://example.com/?a=1&b=2",
			headers:     documentHeaders,
			wantURL:     "https://example.com/?b=2",
			wantApplied: []string{`$doc,removeparam=a`},
		},
		{
			name:    "content type not met",
			rules:   []string{`$doc,removeparam=a`},
			url:     "https://example.com/?a=1&b=2",
			headers: scriptHeaders,
		},
		{
			name:        "regexp",
			rules:       []string{`$removeparam=/^utm_/`},
			url:         "https://example.com/?utm_source=x&utm_medium=y&a=1",
			wantURL:     "https://example.com/?a=1",
			wantApplied: []string{`$removeparam=/^utm_/`},
		},
		{
			// Regexp rules match "name=value", not the name alone.
			name:  "regexp matches name and value",
			rules: []string{`$removeparam=/^a$/`},
			url:   "https://example.com/?a=1",
		},
		{
			name:        "regexp on a single parameter",
			rules:       []string{`$removeparam=/^a=/`},
			url:         "https://example.com/?a=1",
			wantURL:     "https://example.com/",
			wantApplied: []string{`$removeparam=/^a=/`},
		},
		{
			name:    "regexp with domain not met",
			rules:   []string{`$removeparam=/^a=/,domain=site.com`},
			url:     "https://example.com/?a=1",
			headers: http.Header{"Referer": []string{"https://other.com/"}},
		},
		{
			name:        "remove all",
			rules:       []string{`$removeparam`},
			url:         "https://example.com/?a=1&b=2",
			wantURL:     "https://example.com/",
			wantApplied: []string{`$removeparam`},
		},
		{
			name:        "remove all on a single parameter",
			rules:       []string{`$removeparam`},
			url:         "https://example.com/?a=1",
			wantURL:     "https://example.com/",
			wantApplied: []string{`$removeparam`},
		},
		{
			name:    "remove all with content type not met",
			rules:   []string{`$doc,removeparam`},
			url:     "https://example.com/?a=1",
			headers: scriptHeaders,
		},
		{
			name:        "inverse",
			rules:       []string{`$removeparam=~id`},
			url:         "https://example.com/?id=1&a=2",
			wantURL:     "https://example.com/?id=1",
			wantApplied: []string{`$removeparam=~id`},
		},
		{
			name:  "inverse on only its parameter",
			rules: []string{`$removeparam=~id`},
			url:   "https://example.com/?id=1",
		},
		{
			name:        "inverse on a query without its parameter",
			rules:       []string{`$removeparam=~id`},
			url:         "https://example.com/?a=1",
			wantURL:     "https://example.com/",
			wantApplied: []string{`$removeparam=~id`},
		},
		{
			name:    "inverse with domain not met",
			rules:   []string{`$removeparam=~id,domain=site.com`},
			url:     "https://example.com/?a=1",
			headers: http.Header{"Referer": []string{"https://other.com/"}},
		},
		{
			name:        "two parameters in one rule",
			rules:       []string{`$removeparam=a,removeparam=b`},
			url:         "https://example.com/?b=1&c=2",
			wantURL:     "https://example.com/?c=2",
			wantApplied: []string{`$removeparam=a,removeparam=b`},
		},
		{
			// The rule is reported only if it removed the header.
			name:        "rule with an action applies without a query",
			rules:       []string{`$removeparam=a,removeheader=request:X-Test`},
			url:         "https://example.com/",
			headers:     http.Header{"X-Test": []string{"1"}},
			wantApplied: []string{`$removeparam=a,removeheader=request:X-Test`},
		},
		{
			// An exception cancels the whole rule, so it must match even though
			// it has nothing to remove.
			name:    "exception without its parameter cancels a rule with an action",
			rules:   []string{`$removeparam=a,removeheader=request:X-Test`, `@@$removeparam=a`},
			url:     "https://example.com/",
			headers: http.Header{"X-Test": []string{"1"}},
		},
		{
			name:  "exception without its parameter cancels a two-parameter rule",
			rules: []string{`$removeparam=a,removeparam=b`, `@@$removeparam=a`},
			url:   "https://example.com/?b=1&c=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nr := New()
			for _, r := range tt.rules {
				if _, err := nr.ParseRule(r, nil); err != nil {
					t.Fatalf("parse %q: %v", r, err)
				}
			}

			check := func(t *testing.T) {
				t.Helper()

				applied, _, redirectURL := nr.ModifyReq(newTestRequest(t, tt.url, tt.headers.Clone()))
				if redirectURL != tt.wantURL {
					t.Errorf("redirectURL = %q, want %q", redirectURL, tt.wantURL)
				}
				var got []string
				for _, r := range applied {
					got = append(got, r.RawRule)
				}
				if !slices.Equal(got, tt.wantApplied) {
					t.Errorf("applied rules = %q, want %q", got, tt.wantApplied)
				}
			}

			t.Run("before Compact", check)
			nr.Compact()
			t.Run("after Compact", check)
		})
	}
}

func TestPatternlessRemoveParamParsedAfterCompact(t *testing.T) {
	t.Parallel()

	nr := New()
	if _, err := nr.ParseRule(`$removeparam=b`, nil); err != nil {
		t.Fatal(err)
	}
	nr.Compact()
	if _, err := nr.ParseRule(`$removeparam=a`, nil); err != nil {
		t.Fatal(err)
	}

	applied, _, redirectURL := nr.ModifyReq(newTestRequest(t, "https://example.com/?a=1&b=2&c=3", nil))
	if want := "https://example.com/?c=3"; redirectURL != want {
		t.Errorf("redirectURL = %q, want %q", redirectURL, want)
	}
	got := make([]string, 0, len(applied))
	for _, r := range applied {
		got = append(got, r.RawRule)
	}
	if want := []string{`$removeparam=b`, `$removeparam=a`}; !slices.Equal(got, want) {
		t.Errorf("applied rules = %q, want %q", got, want)
	}
}

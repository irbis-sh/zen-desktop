package rulemodifiers

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNewRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		referer   string
		wantHost  string
		wantOK    bool
		wantLabel string
	}{
		{"valid Referer", "https://www.example.co.uk/page", "www.example.co.uk", true, "example"},
		{"Referer with port", "https://example.com:8443/", "example.com", true, "example"},
		{"IPv6 Referer", "https://[::1]:8080/", "::1", true, ""},
		{"no Referer", "", "request.example.com", true, "example"},
		{"Referer without scheme", "example.com/page", "", true, ""},
		{"unparsable Referer", "https://example.com:abc/", "", false, ""},
		{"Referer is a public suffix", "https://co.uk/", "co.uk", true, ""},
		{"Referer without a dot", "http://localhost/", "localhost", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &http.Request{
				URL:    &url.URL{Scheme: "https", Host: "request.example.com:8443"},
				Header: http.Header{},
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}

			got := NewRequest(req)
			if got.sourceHost != tt.wantHost || got.sourceHostOK != tt.wantOK || got.sourceLabel != tt.wantLabel {
				t.Errorf("NewRequest() = (%q, %t, %q), want (%q, %t, %q)",
					got.sourceHost, got.sourceHostOK, got.sourceLabel, tt.wantHost, tt.wantOK, tt.wantLabel)
			}
		})
	}
}

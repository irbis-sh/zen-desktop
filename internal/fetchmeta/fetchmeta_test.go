package fetchmeta_test

import (
	"net/http"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/fetchmeta"
)

func TestIsUserNavigation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		headers map[string]string
		want    bool
	}{
		{
			name:    "document navigation",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "document", "Sec-Fetch-Mode": "navigate"},
			want:    true,
		},
		{
			name:    "user-initiated document without mode",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "document", "Sec-Fetch-User": "?1"},
			want:    true,
		},
		{
			name:    "XHR",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors"},
			want:    false,
		},
		{
			name:    "iframe navigation",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "iframe", "Sec-Fetch-Mode": "navigate"},
			want:    false,
		},
		{
			name:    "no fetch metadata",
			method:  http.MethodGet,
			headers: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := newRequest(t, tt.method, tt.headers)
			if got := fetchmeta.IsUserNavigation(req); got != tt.want {
				t.Errorf("IsUserNavigation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLikelyUserNavigation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		headers map[string]string
		want    bool
	}{
		{
			name:    "document navigation without accept",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "document", "Sec-Fetch-Mode": "navigate"},
			want:    true,
		},
		{
			name:    "XHR asking for HTML",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors", "Accept": "text/html"},
			want:    false,
		},
		{
			name:    "iframe navigation",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "iframe", "Sec-Fetch-Mode": "navigate"},
			want:    true,
		},
		{
			name:    "frame navigation",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "frame", "Sec-Fetch-Mode": "navigate"},
			want:    true,
		},
		{
			name:    "image subresource",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Dest": "image", "Sec-Fetch-Mode": "no-cors"},
			want:    false,
		},
		{
			name:    "form POST navigation",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Dest": "document", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-User": "?1"},
			want:    true,
		},
		{
			name:    "plain-HTTP browser navigation",
			method:  http.MethodGet,
			headers: map[string]string{"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			want:    true,
		},
		{
			name:    "plain-HTTP form POST navigation",
			method:  http.MethodPost,
			headers: map[string]string{"Accept": "text/html"},
			want:    true,
		},
		{
			name:    "no fetch metadata, generic accept",
			method:  http.MethodGet,
			headers: map[string]string{"Accept": "*/*"},
			want:    false,
		},
		{
			name:    "no fetch metadata, no accept",
			method:  http.MethodGet,
			headers: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := newRequest(t, tt.method, tt.headers)
			if got := fetchmeta.IsLikelyUserNavigation(req); got != tt.want {
				t.Errorf("IsLikelyUserNavigation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newRequest(t *testing.T, method string, headers map[string]string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(method, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

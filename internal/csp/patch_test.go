package csp

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testResourceURL = "https://assets.example/resource.js"

func TestPatchHeaders(t *testing.T) {
	t.Parallel()

	resourceURL := testResourceURL

	t.Run("does not create CSP header when none is present", func(t *testing.T) {
		t.Parallel()

		res := &http.Response{Header: http.Header{}, Body: http.NoBody}
		nonce := NewNonce()
		ops := []PatchOperation{{Nonce: nonce, Kind: Script, ResourceURL: resourceURL}}
		err := PatchHeadersBatch(res, ops)
		if err != nil {
			t.Fatalf("patch headers: %v", err)
		}

		if got := res.Header.Values("Content-Security-Policy"); len(got) != 0 {
			t.Fatalf("headers should be unchanged, got %v", got)
		}
	})

	t.Run("always returns a nonce", func(t *testing.T) {
		t.Parallel()

		res := &http.Response{Header: http.Header{}, Body: http.NoBody}
		res.Header.Add("Content-Security-Policy", "script-src 'self'")

		nonce := NewNonce()
		ops := []PatchOperation{{Nonce: nonce, Kind: Script, ResourceURL: testResourceURL}}
		err := PatchHeadersBatch(res, ops)
		if err != nil {
			t.Fatalf("patch headers: %v", err)
		}

		if nonce == "" {
			t.Fatalf("nonce cannot be empty")
		}
		if !dirHasNonce(res.Header, "script-src", nonce) {
			t.Fatalf("expected header to contain nonce; header: %s", res.Header.Get("Content-Security-Policy"))
		}
	})

	t.Run("adds URL when 'unsafe-inline' is present", func(t *testing.T) {
		t.Parallel()

		res := &http.Response{Header: http.Header{}, Body: http.NoBody}
		res.Header.Add("Content-Security-Policy", "script-src 'unsafe-inline'")

		nonce := NewNonce()
		ops := []PatchOperation{{Nonce: nonce, Kind: Script, ResourceURL: testResourceURL}}
		err := PatchHeadersBatch(res, ops)
		if err != nil {
			t.Fatalf("patch headers: %v", err)
		}
		if nonce == "" {
			t.Fatalf("nonce cannot be empty")
		}

		got := res.Header.Get("Content-Security-Policy")
		if !strings.Contains(got, testResourceURL) {
			t.Fatalf("expected header to contain resource URL; header: %s", got)
		}
	})

	t.Run("replace 'none' in most specific", func(t *testing.T) {
		t.Parallel()

		res := &http.Response{Header: http.Header{}, Body: http.NoBody}
		res.Header.Add("Content-Security-Policy", "script-src-elem 'none'")

		nonce := NewNonce()
		ops := []PatchOperation{{Nonce: nonce, Kind: Script, ResourceURL: testResourceURL}}
		err := PatchHeadersBatch(res, ops)
		if err != nil {
			t.Fatalf("patch headers: %v", err)
		}
		token := "'nonce-" + nonce + "'"

		got := strings.Join(res.Header.Values("Content-Security-Policy"), ", ")
		expected := fmt.Sprintf("script-src-elem %s", token)
		if got != expected {
			t.Fatalf("expected header value %q, got %q", expected, got)
		}
	})
}

func TestPatchHeaders_NoncePriority_Script(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		cspLine       string
		wantDirective string
	}{
		{
			name:          "script-src-elem is most specific",
			cspLine:       "default-src 'self'; script-src 'self'; script-src-elem 'self'",
			wantDirective: "script-src-elem",
		},
		{
			name:          "script-src fallback",
			cspLine:       "object-src 'none'; script-src 'self'",
			wantDirective: "script-src",
		},
		{
			name:          "default-src fallback",
			cspLine:       "default-src 'self'",
			wantDirective: "default-src",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res := &http.Response{Header: http.Header{}, Body: http.NoBody}
			res.Header.Add("Content-Security-Policy", tc.cspLine)

			nonce := NewNonce()
			ops := []PatchOperation{{Nonce: nonce, Kind: Script, ResourceURL: "https://assets.example/one.js"}}
			err := PatchHeadersBatch(res, ops)
			if err != nil {
				t.Fatalf("patch headers: %v", err)
			}
			if !dirHasNonce(res.Header, tc.wantDirective, nonce) {
				t.Fatalf("nonce not placed in %s\nheader: %s",
					tc.wantDirective, res.Header.Get("Content-Security-Policy"))
			}
		})
	}
}

func TestPatchHeaders_NoncePriority_Style(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		cspLine       string
		wantDirective string
	}{
		{
			name:          "style-src-elem is most specific",
			cspLine:       "default-src 'self'; style-src 'self'; style-src-elem 'self'",
			wantDirective: "style-src-elem",
		},
		{
			name:          "style-src fallback",
			cspLine:       "object-src 'none'; style-src 'self'",
			wantDirective: "style-src",
		},
		{
			name:          "default-src fallback",
			cspLine:       "default-src 'self'",
			wantDirective: "default-src",
		},
	}

	for _, tc := range cases {
		res := &http.Response{Header: http.Header{}, Body: http.NoBody}
		res.Header.Add("Content-Security-Policy", tc.cspLine)

		nonce := NewNonce()
		ops := []PatchOperation{{Nonce: nonce, Kind: Style, ResourceURL: "https://assets.example/one.css"}}
		err := PatchHeadersBatch(res, ops)
		if err != nil {
			t.Fatalf("patch headers: %v", err)
		}

		token := "'nonce-" + nonce + "'"
		found := false
		for _, line := range res.Header.Values("Content-Security-Policy") {
			if strings.Contains(strings.ToLower(line), tc.wantDirective) && strings.Contains(line, token) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: nonce not placed in %s; header: %s", tc.name, tc.wantDirective, strings.Join(res.Header.Values("Content-Security-Policy"), " | "))
		}
	}
}

func TestPatchHeaders_Meta(t *testing.T) {
	t.Parallel()

	t.Run("meta only", func(t *testing.T) {
		t.Parallel()

		htmlBody := `<html><head><meta http-equiv="Content-Security-Policy" content="script-src 'none'"></head><body></body></html>`
		res := &http.Response{
			Header: http.Header{},
			Body:   io.NopCloser(strings.NewReader(htmlBody)),
		}
		res.Header.Set("Content-Type", "text/html; charset=utf-8")

		nonce := NewNonce()
		ops := []PatchOperation{{Nonce: nonce, Kind: Script, ResourceURL: testResourceURL}}
		err := PatchHeadersBatch(res, ops)
		if err != nil {
			t.Fatalf("patch headers: %v", err)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		res.Body.Close()

		token := "'nonce-" + nonce + "'"
		escapedToken := strings.ReplaceAll(token, "'", "&#39;")
		bodyStr := string(body)
		if !strings.Contains(bodyStr, token) && !strings.Contains(bodyStr, escapedToken) {
			t.Fatalf("expected meta CSP to contain %q (or %q), body: %s", token, escapedToken, bodyStr)
		}
	})

	t.Run("header and meta", func(t *testing.T) {
		t.Parallel()

		htmlBody := `<html><head><meta http-equiv="Content-Security-Policy" content="script-src 'none'"></head><body></body></html>`
		res := &http.Response{
			Header: http.Header{},
			Body:   io.NopCloser(strings.NewReader(htmlBody)),
		}
		res.Header.Set("Content-Type", "text/html; charset=utf-8")
		res.Header.Add("Content-Security-Policy", "script-src 'none'")

		nonce := NewNonce()
		ops := []PatchOperation{{Nonce: nonce, Kind: Script, ResourceURL: testResourceURL}}
		err := PatchHeadersBatch(res, ops)
		if err != nil {
			t.Fatalf("patch headers: %v", err)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		res.Body.Close()

		token := "'nonce-" + nonce + "'"
		escapedToken := strings.ReplaceAll(token, "'", "&#39;")
		bodyStr := string(body)
		if !strings.Contains(bodyStr, token) && !strings.Contains(bodyStr, escapedToken) {
			t.Fatalf("expected meta CSP to contain %q (or %q), body: %s", token, escapedToken, bodyStr)
		}

		if !dirHasNonce(res.Header, "script-src", nonce) {
			t.Fatalf("expected header to contain nonce; header: %s", res.Header.Get("Content-Security-Policy"))
		}
	})
}

func dirHasNonce(h http.Header, dir, nonce string) bool {
	token := "'nonce-" + nonce + "'"
	lines := h.Values("Content-Security-Policy")

	for _, line := range lines {
		rawDirs := strings.SplitSeq(line, ";")

		for raw := range rawDirs {
			d := strings.TrimSpace(raw)
			if d == "" {
				continue
			}
			name, value := cutDirective(d)
			if name == dir && strings.Contains(value, token) {
				return true
			}
		}
	}
	return false
}

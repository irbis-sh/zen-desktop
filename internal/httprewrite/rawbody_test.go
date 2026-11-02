package httprewrite_test

import (
	"io"
	"strings"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/httprewrite"
)

func TestBufferRewriteKeepsEncoding(t *testing.T) {
	t.Parallel()

	// The first 1024 bytes are ASCII, so a guess from them would pick Windows-1252.
	original := `{"text":"` + strings.Repeat("x", 1100) + `é 😀"}`
	res := newGzipHTTPResponse(t, "application/json", []byte(original))

	err := httprewrite.BufferRewrite(res, func(src []byte) []byte { return src })
	if err != nil {
		t.Fatalf("BufferRewrite error: %v", err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != original {
		t.Errorf("body = %q, want %q", body, original)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if ce := res.Header.Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want none", ce)
	}
}

func TestIsUTF8Script(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		want   bool
	}{
		{"ASCII", `var a = "b";`, true},
		{"UTF-8", `var a = "表示 😀";`, true},
		{"GBK", "var a = \"\xd6\xd0\xce\xc4\";", false},
		{"Shift_JIS with a backslash second byte", "var a = \"\x95\x5c\";", false},
		{"ISO-2022-JP", "var a = \"\x1b$BI=<(\x1b(B\";", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := httprewrite.IsUTF8Script([]byte(tt.script)); got != tt.want {
				t.Errorf("IsUTF8Script(%q) = %v, want %v", tt.script, got, tt.want)
			}
		})
	}
}

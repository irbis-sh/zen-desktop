package removejsconstant_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/rulemodifiers/removejsconstant"
)

func TestModifyResEncodings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name:        "UTF-8 JS",
			contentType: "text/javascript",
			body:        `var cfg = {ad: 1, text: "表示"};`,
			want:        `var cfg = {text: "表示"};`,
		},
		{
			name:        "UTF-8 inline script",
			contentType: "text/html",
			body:        `<html><head><script>var cfg = {ad: 1, text: "表示"};</script></head></html>`,
			want:        `<html><head><script>var cfg = {text: "表示"};</script></head></html>`,
		},
		{
			// "表示". The parser reads 0x5C 0x8E as an escape, so without the UTF-8 check
			// the script would parse and be rewritten, with "ad" removed.
			name:        "Shift_JIS JS",
			contentType: "text/javascript",
			body:        "var cfg = {ad: 1, text: \"\x95\x5c\x8e\xa6\"};",
			want:        "var cfg = {ad: 1, text: \"\x95\x5c\x8e\xa6\"};",
		},
		{
			name:        "ISO-2022-JP JS",
			contentType: "text/javascript",
			body:        "var cfg = {ad: 1, text: \"\x1b$BI=<(\x1b(B\"};",
			want:        "var cfg = {ad: 1, text: \"\x1b$BI=<(\x1b(B\"};",
		},
		{
			name:        "Shift_JIS inline script",
			contentType: "text/html",
			body:        "<html><head><script>var cfg = {ad: 1, text: \"\x95\x5c\x8e\xa6\"};</script></head></html>",
			want:        "<html><head><script>var cfg = {ad: 1, text: \"\x95\x5c\x8e\xa6\"};</script></head></html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var m removejsconstant.Modifier
			if err := m.Parse("remove-js-constant=cfg.ad"); err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			res := newHTTPResponse(tt.contentType, []byte(tt.body))
			if _, err := m.ModifyRes(res); err != nil {
				t.Fatalf("ModifyRes error: %v", err)
			}
			got, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("body = %q, want %q", got, tt.want)
			}
		})
	}
}

func newHTTPResponse(contentType string, body []byte) *http.Response {
	return &http.Response{
		Body:       io.NopCloser(bytes.NewReader(body)),
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{contentType},
		},
		Request: &http.Request{},
	}
}

package httprewrite_test

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/irbis-sh/zen-desktop/internal/httprewrite"
)

func TestAppendHTMLHeadContentsPublic(t *testing.T) {
	t.Parallel()

	type tc struct {
		name       string
		original   []byte
		appendWith []byte
		expected   []byte
	}

	tests := make([]tc, 0, 4)
	tests = append(tests,
		tc{
			"appends <head> contents",
			[]byte(`<html><head>Original Head Content</head><body>Test</body></html>`),
			[]byte("-Appended"),
			[]byte(`<html><head>Original Head Content-Appended</head><body>Test</body></html>`),
		},
		tc{
			"doesn't modify body on empty appendWith",
			[]byte(`<html><head>Original Head Content</head></html>`),
			[]byte(""),
			[]byte(`<html><head>Original Head Content</head></html>`),
		},
		tc{
			"doesn't modify response if no <head> is present",
			[]byte(`<html><body>Test</body></html>`),
			[]byte("test"),
			[]byte(`<html><body>Test</body></html>`),
		},
		tc{
			"doesn't modify empty response",
			[]byte(""),
			[]byte("test"),
			[]byte(""),
		},
	)

	generatedBytes := genAlphanumByteArray(10 * 1024 * 1024) // 10MB
	tests = append(tests, tc{
		name:       "appends to large <head> contents",
		original:   bytes.Join([][]byte{[]byte(`<html><head>`), generatedBytes, []byte(`</head></html>`)}, nil),
		appendWith: []byte("-Appended"),
		expected:   bytes.Join([][]byte{[]byte(`<html><head>`), generatedBytes, []byte("-Appended"), []byte(`</head></html>`)}, nil),
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := newHTTPResponse(tt.original)
			if err := httprewrite.AppendHTMLHeadContents(res, tt.appendWith); err != nil {
				t.Fatalf("AppendHeadContents error: %v", err)
			}
			modifiedBody, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("failed to read modified body: %v", err)
			}
			if !bytes.Equal(modifiedBody, tt.expected) {
				if len(modifiedBody) < 1024 && len(tt.expected) < 1024 {
					t.Errorf("expected response body %q, got %q", tt.expected, modifiedBody)
				} else {
					t.Error("expected body != modifiedBody")
				}
			}
		})
	}
}

func TestAppendHTMLHeadContentsKeepsEncoding(t *testing.T) {
	t.Parallel()

	const injection = `<script src="https://zen.local/a.js"></script>`
	// Pushes whatever follows past the first 1024 bytes, where browsers stop looking for <meta charset>.
	padding := "<!--" + strings.Repeat("x", 1100) + "-->"
	gbkText := "\xd6\xd0\xce\xc4"       // "中文"
	iso2022JPText := "\x1b$BI=<(\x1b(B" // "表示"; note the '<' inside

	tests := []struct {
		name        string
		contentType string
		head        string
		body        string
	}{
		{
			name:        "GBK declared in <meta>",
			contentType: "text/html",
			head:        `<html><head><meta charset="gbk"><title>` + gbkText + `</title>`,
			body:        `<body>` + gbkText + `</body></html>`,
		},
		{
			name:        "GBK declared in <meta> after the first 1024 bytes",
			contentType: "text/html",
			head:        `<html><head>` + padding + `<meta charset="gbk"><title>` + gbkText + `</title>`,
			body:        `<body>` + gbkText + `</body></html>`,
		},
		{
			name:        "GBK declared in header",
			contentType: "text/html; charset=gbk",
			head:        `<html><head><title>` + gbkText + `</title>`,
			body:        `<body>` + gbkText + `</body></html>`,
		},
		{
			name:        "ISO-2022-JP declared in header",
			contentType: "text/html; charset=iso-2022-jp",
			head:        `<html><head><title>` + iso2022JPText + `</title>`,
			body:        `<body>` + iso2022JPText + `</body></html>`,
		},
		{
			// Browsers let the byte order mark win, so the page is UTF-8.
			name:        "UTF-8 byte order mark with a contradicting UTF-16 header",
			contentType: "text/html; charset=utf-16le",
			head:        "\xef\xbb\xbf<html><head><title>中文</title>",
			body:        `<body>中文</body></html>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := tt.head + "</head>" + tt.body
			res := newGzipHTTPResponse(t, tt.contentType, []byte(original))
			if err := httprewrite.AppendHTMLHeadContents(res, []byte(injection)); err != nil {
				t.Fatalf("AppendHTMLHeadContents error: %v", err)
			}

			got, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if want := tt.head + injection + "</head>" + tt.body; string(got) != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if ct := res.Header.Get("Content-Type"); ct != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", ct, tt.contentType)
			}
			if ce := res.Header.Get("Content-Encoding"); ce != "" {
				t.Errorf("Content-Encoding = %q, want none", ce)
			}
		})
	}
}

func TestAppendHTMLHeadContentsConvertsUTF16(t *testing.T) {
	t.Parallel()

	const (
		injection = `<script src="https://zen.local/a.js"></script>`
		head      = `<html><head><title>中文 😀</title>`
		body      = `<body>中文 😀</body></html>`
	)

	tests := []struct {
		name        string
		contentType string
		order       binary.ByteOrder
		bom         string
	}{
		{
			name:        "UTF-16LE byte order mark",
			contentType: "text/html",
			order:       binary.LittleEndian,
			bom:         "\ufeff",
		},
		{
			name:        "UTF-16BE byte order mark",
			contentType: "text/html",
			order:       binary.BigEndian,
			bom:         "\ufeff",
		},
		{
			name:        "utf-16le header label",
			contentType: "text/html; charset=utf-16le",
			order:       binary.LittleEndian,
		},
		{
			name:        "utf-16be header label",
			contentType: "text/html; charset=utf-16be",
			order:       binary.BigEndian,
		},
		{
			// Browsers read a bare utf-16 label as UTF-16LE.
			name:        "utf-16 header label",
			contentType: "text/html; charset=utf-16",
			order:       binary.LittleEndian,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := tt.bom + head + "</head>" + body
			res := newGzipHTTPResponse(t, tt.contentType, encodeUTF16(original, tt.order))
			if err := httprewrite.AppendHTMLHeadContents(res, []byte(injection)); err != nil {
				t.Fatalf("AppendHTMLHeadContents error: %v", err)
			}

			got, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if want := tt.bom + head + injection + "</head>" + body; string(got) != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Errorf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
			}
		})
	}
}

func newHTTPResponse(body []byte) *http.Response {
	return &http.Response{
		Body:       io.NopCloser(bytes.NewReader(body)),
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
	}
}

func genAlphanumByteArray(length int) []byte {
	const alphanumerics = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	alphanumLen := len(alphanumerics)
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		result[i] = alphanumerics[rand.Intn(alphanumLen)] // #nosec G404 -- not used for security purposes
	}

	return result
}

func newGzipHTTPResponse(t *testing.T, contentType string, body []byte) *http.Response {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	return &http.Response{
		Body:       io.NopCloser(&buf),
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":     []string{contentType},
			"Content-Encoding": []string{"gzip"},
		},
	}
}

func encodeUTF16(s string, order binary.ByteOrder) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(units))
	for i, u := range units {
		order.PutUint16(b[2*i:], u)
	}
	return b
}

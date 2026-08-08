package localcdn

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func filterTestMatch(rawURL string) bool {
	return strings.Contains(rawURL, "ajax.googleapis.com")
}

func rewriteTestHTML(t *testing.T, body string) (string, error) {
	t.Helper()
	res := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	if err := FilterHTML(res, filterTestMatch); err != nil {
		return "", err
	}
	out, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func TestFilterHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		absent  []string
		present []string
	}{
		{
			name:    "script with integrity and crossorigin",
			in:      `<script src="https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js" integrity="sha384-abc" crossorigin="anonymous"></script>`,
			absent:  []string{"integrity", "crossorigin"},
			present: []string{"https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js"},
		},
		{
			name:    "crossorigin without integrity",
			in:      `<script src="https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js" crossorigin></script>`,
			absent:  []string{"crossorigin"},
			present: []string{"jquery.min.js"},
		},
		{
			name:    "stylesheet link",
			in:      `<link rel="stylesheet" href="https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/theme.css" integrity="sha384-xyz" crossorigin="anonymous">`,
			absent:  []string{"integrity", "crossorigin"},
			present: []string{`rel="stylesheet"`},
		},
		{
			name:    "non-matching URL keeps attributes",
			in:      `<script src="https://example.com/app.js" integrity="sha384-abc" crossorigin="anonymous"></script>`,
			absent:  nil,
			present: []string{"integrity", "crossorigin"},
		},
		{
			name:    "already stripped tag",
			in:      `<script src="https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js"></script>`,
			absent:  nil,
			present: []string{"jquery.min.js"},
		},
		{
			name:    "single quotes and uppercase attributes",
			in:      `<script src='https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js' INTEGRITY='sha384-abc' CrossOrigin='anonymous'></script>`,
			absent:  []string{"INTEGRITY", "CrossOrigin"},
			present: []string{"jquery.min.js"},
		},
		{
			name:    "no relevant attributes leaves body unchanged",
			in:      `<html><head><title>Test</title></head><body><p>Hello</p></body></html>`,
			absent:  nil,
			present: []string{`<html><head><title>Test</title></head><body><p>Hello</p></body></html>`},
		},
		{
			name:    "malformed html with complete start tag",
			in:      `<div><script src="https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js" integrity="sha384-abc"></div>`,
			absent:  []string{"integrity"},
			present: []string{"jquery.min.js"},
		},
		{
			name:    "style tag without url is untouched",
			in:      `<style integrity="sha384-abc">body{color:red}</style>`,
			absent:  nil,
			present: []string{"integrity"},
		},
		{
			name:    "relative url is untouched",
			in:      `<script src="/js/app.js" integrity="sha384-abc"></script>`,
			absent:  nil,
			present: []string{"integrity"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := rewriteTestHTML(t, tt.in)
			if err != nil {
				t.Fatalf("FilterHTML: %v", err)
			}
			for _, absent := range tt.absent {
				if strings.Contains(got, absent) {
					t.Fatalf("output contains %q:\n%s", absent, got)
				}
			}
			for _, present := range tt.present {
				if !strings.Contains(got, present) {
					t.Fatalf("output does not contain %q:\n%s", present, got)
				}
			}
		})
	}
}

func TestFilterHTMLUnclosedTagDoesNotHang(t *testing.T) {
	t.Parallel()

	body := `<script src="https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js" integrity="sha384-abc`
	if _, err := rewriteTestHTML(t, body); err != nil {
		t.Fatalf("FilterHTML: %v", err)
	}
}

func TestStripAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "removes both attributes",
			in:   `<script src="x.js" integrity="sha384-abc" crossorigin></script>`,
			want: `<script src="x.js"></script>`,
		},
		{
			name: "keeps other attributes",
			in:   `<script async defer src="x.js" integrity="sha384-abc"></script>`,
			want: `<script async defer src="x.js"></script>`,
		},
		{
			name: "removes nothing when absent",
			in:   `<script src="x.js"></script>`,
			want: `<script src="x.js"></script>`,
		},
		{
			name: "unquoted value",
			in:   `<script src="x.js" integrity=sha384-abc></script>`,
			want: `<script src="x.js"></script>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := string(stripAttributes([]byte(tt.in), "integrity", "crossorigin")); got != tt.want {
				t.Fatalf("stripAttributes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

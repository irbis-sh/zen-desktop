package networkrules

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRegexpRules(t *testing.T) {
	t.Parallel()

	t.Run("blocks matching request", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/ads\d+\.js/`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/ads123.js", nil))
		if !shouldBlock {
			t.Fatal("expected rule to block matching request")
		}
	})

	t.Run("does not block nonmatching request", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/ads\d+\.js/`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/ad.js", nil))
		if shouldBlock {
			t.Fatal("expected rule not to block nonmatching request")
		}
	})

	t.Run("respects request modifiers", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/\/[0-9a-f]{32}\/invoke\.js/$script,third-party,domain=metbuat.az`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{
			"Referer":        []string{"https://metbuat.az/page"},
			"Sec-Fetch-Dest": []string{"script"},
			"Sec-Fetch-Site": []string{"cross-site"},
		}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://cdn.example/0123456789abcdef0123456789abcdef/invoke.js", headers))
		if !shouldBlock {
			t.Fatal("expected rule to block when modifiers match")
		}

		headers.Set("Sec-Fetch-Dest", "image")
		_, shouldBlock, _ = nr.ModifyReq(newTestRequest(t, "https://cdn.example/0123456789abcdef0123456789abcdef/invoke.js", headers))
		if shouldBlock {
			t.Fatal("expected rule not to block when content type modifier fails")
		}
	})

	t.Run("matches unescaped slash in regexp body", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/example.com/[0-9a-z]*.php/$domain=example.com`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{"Referer": []string{"https://example.com/page"}}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://cdn.example/example.com/abc123.php", headers))
		if !shouldBlock {
			t.Fatal("expected rule with unescaped slash to block matching request")
		}
	})

	t.Run("regexp exception cancels tree primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com/ads*`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@/ads\d+/`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/ads123", nil))
		if shouldBlock {
			t.Fatal("expected regexp exception to cancel tree primary rule")
		}
	})

	t.Run("tree exception cancels regexp primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/ads\d+/`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com/ads*`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/ads123", nil))
		if shouldBlock {
			t.Fatal("expected tree exception to cancel regexp primary rule")
		}
	})

	t.Run("invalid regexp returns error", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/[/$script`, nil); err == nil {
			t.Fatal("expected invalid rule to return error")
		}
	})

	t.Run("empty regexp returns error", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`//`, nil); err == nil {
			t.Fatal("expected empty rule to return error")
		}
	})

	t.Run("unsupported lookahead regexp returns error", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/demo.example\/(?!.*animated).*\.gif/$domain=demo.example`, nil); err == nil {
			t.Fatal("expected lookahead rule to return error")
		}
	})

	t.Run("unsupported backreference regexp returns error", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/^https:\/\/(?:loader)\.([-0-9A-Za-z]+\.[-A-Za-z]{2,16})\/(?:loader\.min|script\/(?:www\.)?\1)\.js$/$script,third-party`, nil); err == nil {
			t.Fatal("expected backreference rule to return error")
		}
	})

	t.Run("modifies matching response", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`/tracking\.js/$removeheader=X-Test`, nil); err != nil {
			t.Fatal(err)
		}

		res := &http.Response{Header: http.Header{"X-Test": []string{"1"}}}
		applied, err := nr.ModifyRes(newTestRequest(t, "https://example.com/tracking.js", nil), res)
		if err != nil {
			t.Fatal(err)
		}
		if len(applied) != 1 {
			t.Fatalf("applied rules = %d, want 1", len(applied))
		}
		if res.Header.Get("X-Test") != "" {
			t.Fatal("expected response header to be removed")
		}
	})
}

func newTestRequest(t *testing.T, rawURL string, headers http.Header) *http.Request {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if headers == nil {
		headers = http.Header{}
	}

	return &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Header: headers,
	}
}

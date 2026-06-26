package networkrules

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHostsRules(t *testing.T) {
	t.Parallel()

	t.Run("blocks matching index request", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`0.0.0.0 example.com`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com", nil))
		if !shouldBlock {
			t.Fatal("expected rule to block matching request")
		}
	})

	t.Run("blocks matching non-index request", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`0.0.0.0 example.com`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/test", nil))
		if !shouldBlock {
			t.Fatal("expected rule to block matching request")
		}
	})

	t.Run("blocks matching sub-domain", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`0.0.0.0 example.com`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://sub.example.com", nil))
		if !shouldBlock {
			t.Fatal("expected rule to block matching request")
		}
	})
}

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

func TestExceptionRules(t *testing.T) {
	t.Parallel()

	t.Run("generic exception cancels document primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$document`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{
			"Sec-Fetch-Dest": []string{"document"},
			"Sec-Fetch-User": []string{"?1"},
		}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/", headers))
		if shouldBlock {
			t.Fatal("expected generic exception to cancel document primary rule")
		}
	})

	t.Run("exception cancels primary with matching request modifiers", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||cdn.example/ad.js$script,domain=example.com`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||cdn.example/ad.js$script,domain=example.com`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{
			"Referer":        []string{"https://example.com/page"},
			"Sec-Fetch-Dest": []string{"script"},
		}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://cdn.example/ad.js", headers))
		if shouldBlock {
			t.Fatal("expected exception to cancel primary rule with matching request modifiers")
		}
	})

	t.Run("exception does not cancel a different content type", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com/ad$script`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com/ad$image`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{"Sec-Fetch-Dest": []string{"script"}}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/ad", headers))
		if !shouldBlock {
			t.Fatal("expected primary rule to block when exception is for a different content type")
		}
	})

	t.Run("single exception cancels multiple primary rules", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com/ad.js$script`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`||example.com/ad.js$script,third-party`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com/ad.js$script`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{
			"Sec-Fetch-Dest": []string{"script"},
			"Sec-Fetch-Site": []string{"cross-site"},
		}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/ad.js", headers))
		if shouldBlock {
			t.Fatal("expected single exception to cancel every matching primary rule")
		}
	})

	t.Run("exception cancels query modification", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$removeparam=utm_source`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^$removeparam=utm_source`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, redirectURL := nr.ModifyReq(newTestRequest(t, "https://example.com/?utm_source=ad&id=1", nil))
		if shouldBlock {
			t.Fatal("expected query modification rule not to block")
		}
		if redirectURL != "" {
			t.Fatalf("redirect URL = %q, want empty", redirectURL)
		}
	})

	t.Run("exception cancels response modification", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com/tracking.js$removeheader=X-Test`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com/tracking.js$removeheader=X-Test`, nil); err != nil {
			t.Fatal(err)
		}

		res := &http.Response{Header: http.Header{"X-Test": []string{"1"}}}
		applied, err := nr.ModifyRes(newTestRequest(t, "https://example.com/tracking.js", nil), res)
		if err != nil {
			t.Fatal(err)
		}
		if len(applied) != 0 {
			t.Fatalf("applied rules = %d, want 0", len(applied))
		}
		if res.Header.Get("X-Test") != "1" {
			t.Fatal("expected response header to be preserved")
		}
	})

	t.Run("normal exception does not cancel important primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$important`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/", nil))
		if !shouldBlock {
			t.Fatal("expected normal exception NOT to cancel important primary rule")
		}
	})

	t.Run("important exception cancels important primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$important`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^$important`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/", nil))
		if shouldBlock {
			t.Fatal("expected important exception to cancel important primary rule")
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

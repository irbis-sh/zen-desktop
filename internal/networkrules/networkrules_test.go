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

	t.Run("all exception cancels all primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$all`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^$all`, nil); err != nil {
			t.Fatal(err)
		}

		navigationHeaders := http.Header{
			"Sec-Fetch-Dest": []string{"document"},
			"Sec-Fetch-User": []string{"?1"},
		}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/", navigationHeaders))
		if shouldBlock {
			t.Error("expected all exception to cancel all primary rule on user navigation")
		}

		scriptHeaders := http.Header{"Sec-Fetch-Dest": []string{"script"}}
		_, shouldBlock, _ = nr.ModifyReq(newTestRequest(t, "https://example.com/script.js", scriptHeaders))
		if shouldBlock {
			t.Error("expected all exception to cancel all primary rule on subresource")
		}
	})

	t.Run("all exception cancels non-document primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$script`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^$all`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{"Sec-Fetch-Dest": []string{"script"}}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/script.js", headers))
		if shouldBlock {
			t.Fatal("expected all exception to cancel non-document primary rule")
		}
	})

	t.Run("document exception cancels all primary", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$all`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^$document`, nil); err != nil {
			t.Fatal(err)
		}

		headers := http.Header{
			"Sec-Fetch-Dest": []string{"document"},
			"Sec-Fetch-User": []string{"?1"},
		}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/", headers))
		if shouldBlock {
			t.Fatal("expected document exception to cancel all primary rule")
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

	t.Run("block-only exception cancels response modification", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com/tracking.js$removeheader=X-Test`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com/tracking.js`, nil); err != nil {
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

	t.Run("action exception does not unblock", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := nr.ParseRule(`@@||example.com^$removeparam=utm_source`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/?utm_source=ad", nil))
		if !shouldBlock {
			t.Fatal("expected action exception NOT to cancel blocking primary rule")
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

func TestExceptionConditions(t *testing.T) {
	t.Parallel()

	moatRules := []string{`||moatads.com^`, `@@||moatads.com^$script,domain=imsa.com|nascar.com`}

	tests := []struct {
		name         string
		rules        []string
		url          string
		headers      http.Header
		wantBlock    bool
		wantRedirect string
	}{
		{
			name:  "scoped exception cancels broader primary",
			rules: moatRules,
			url:   "https://z.moatads.com/x.js",
			headers: http.Header{
				"Referer":        []string{"https://www.nascar.com/"},
				"Sec-Fetch-Dest": []string{"script"},
			},
			wantBlock: false,
		},
		{
			name:  "scoped exception does not apply from another domain",
			rules: moatRules,
			url:   "https://z.moatads.com/x.js",
			headers: http.Header{
				"Referer":        []string{"https://other.example/"},
				"Sec-Fetch-Dest": []string{"script"},
			},
			wantBlock: true,
		},
		{
			name:  "scoped exception does not apply to another content type",
			rules: moatRules,
			url:   "https://z.moatads.com/x.gif",
			headers: http.Header{
				"Referer":        []string{"https://www.nascar.com/"},
				"Sec-Fetch-Dest": []string{"image"},
			},
			wantBlock: true,
		},
		{
			name:      "exception with fewer domains cancels primary on its domain",
			rules:     []string{`||ads.example^$domain=a.com|b.com`, `@@||ads.example^$domain=a.com`},
			url:       "https://ads.example/",
			headers:   http.Header{"Referer": []string{"https://a.com/"}},
			wantBlock: false,
		},
		{
			name:      "exception with fewer domains does not cancel primary on other domains",
			rules:     []string{`||ads.example^$domain=a.com|b.com`, `@@||ads.example^$domain=a.com`},
			url:       "https://ads.example/",
			headers:   http.Header{"Referer": []string{"https://b.com/"}},
			wantBlock: true,
		},
		{
			name:      "third-party exception cancels primary on cross-site request",
			rules:     []string{`||ads.example^`, `@@||ads.example^$third-party`},
			url:       "https://ads.example/",
			headers:   http.Header{"Sec-Fetch-Site": []string{"cross-site"}},
			wantBlock: false,
		},
		{
			name:      "third-party exception does not cancel primary on same-origin request",
			rules:     []string{`||ads.example^`, `@@||ads.example^$third-party`},
			url:       "https://ads.example/",
			headers:   http.Header{"Sec-Fetch-Site": []string{"same-origin"}},
			wantBlock: true,
		},
		{
			name:    "scoped action exception cancels query modification on its domain",
			rules:   []string{`||example.com^$removeparam=utm_source`, `@@||example.com^$removeparam=utm_source,domain=a.com`},
			url:     "https://example.com/?utm_source=ad&id=1",
			headers: http.Header{"Referer": []string{"https://a.com/"}},
		},
		{
			name:         "scoped action exception does not cancel query modification on other domains",
			rules:        []string{`||example.com^$removeparam=utm_source`, `@@||example.com^$removeparam=utm_source,domain=a.com`},
			url:          "https://example.com/?utm_source=ad&id=1",
			headers:      http.Header{"Referer": []string{"https://b.com/"}},
			wantRedirect: "https://example.com/?id=1",
		},
		{
			// $domain is read from the Referer, so on a clicked navigation it names the page the
			// click came from. AdGuard does the same; uBO checks the page being opened and would block.
			name:  "scoped exception cancels document primary on navigation from its domain",
			rules: []string{`||tracker.example^$document`, `@@||tracker.example^$domain=a.com`},
			url:   "https://tracker.example/",
			headers: http.Header{
				"Referer":        []string{"https://a.com/"},
				"Sec-Fetch-Dest": []string{"document"},
				"Sec-Fetch-User": []string{"?1"},
			},
			wantBlock: false,
		},
		{
			name:      "scoped exception does not cancel important primary",
			rules:     []string{`||example.com^$important`, `@@||example.com^$domain=a.com`},
			url:       "https://example.com/",
			headers:   http.Header{"Referer": []string{"https://a.com/"}},
			wantBlock: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nr := New()
			for _, r := range tt.rules {
				if _, err := nr.ParseRule(r, nil); err != nil {
					t.Fatal(err)
				}
			}

			_, shouldBlock, redirectURL := nr.ModifyReq(newTestRequest(t, tt.url, tt.headers))
			if shouldBlock != tt.wantBlock {
				t.Errorf("shouldBlock = %t, want %t", shouldBlock, tt.wantBlock)
			}
			if redirectURL != tt.wantRedirect {
				t.Errorf("redirect URL = %q, want %q", redirectURL, tt.wantRedirect)
			}
		})
	}

	// In ModifyRes, $domain, $third-party and $method never match, so only exceptions
	// conditioned on content type or $header apply to responses.
	t.Run("only content type and header conditions apply to responses", func(t *testing.T) {
		t.Parallel()

		for exception, wantHeader := range map[string]string{
			`@@||cdn.example^$removeheader=X-Test,script`:       "1",
			`@@||cdn.example^$removeheader=X-Test,domain=a.com`: "",
		} {
			nr := New()
			for _, r := range []string{`||cdn.example^$removeheader=X-Test`, exception} {
				if _, err := nr.ParseRule(r, nil); err != nil {
					t.Fatal(err)
				}
			}

			res := &http.Response{Header: http.Header{
				"Content-Type": []string{"text/javascript"},
				"X-Test":       []string{"1"},
			}}
			if _, err := nr.ModifyRes(newTestRequest(t, "https://cdn.example/x.js", nil), res); err != nil {
				t.Fatal(err)
			}
			if got := res.Header.Get("X-Test"); got != wantHeader {
				t.Errorf("with %s: X-Test = %q, want %q", exception, got, wantHeader)
			}
		}
	})
}

func TestContentTypes(t *testing.T) {
	t.Parallel()

	clickedLinkHeaders := http.Header{
		"Sec-Fetch-Dest": []string{"document"},
		"Sec-Fetch-User": []string{"?1"},
	}
	negatedRules := []string{`/adserver.$~stylesheet,~xmlhttprequest`}

	tests := []struct {
		name      string
		rules     []string
		url       string
		headers   http.Header
		wantBlock bool
	}{
		{
			name:      "document rule blocks clicked link",
			rules:     []string{`||example.com^$document`},
			url:       "https://example.com/",
			headers:   clickedLinkHeaders,
			wantBlock: true,
		},
		{
			name:    "document rule does not block image",
			rules:   []string{`||example.com^$document`},
			url:     "https://example.com/pixel.png",
			headers: http.Header{"Sec-Fetch-Dest": []string{"image"}},
		},
		{
			name:    "other rule does not block script-driven page load",
			rules:   []string{`||example.com^$other`},
			url:     "https://example.com/",
			headers: http.Header{"Sec-Fetch-Dest": []string{"document"}, "Sec-Fetch-Mode": []string{"navigate"}},
		},
		{
			name:      "hosts entry blocks clicked link",
			rules:     []string{`0.0.0.0 example.com`},
			url:       "https://example.com/",
			headers:   clickedLinkHeaders,
			wantBlock: true,
		},
		{
			name:      "hosts entry blocks script",
			rules:     []string{`0.0.0.0 example.com`},
			url:       "https://example.com/script.js",
			headers:   http.Header{"Sec-Fetch-Dest": []string{"script"}},
			wantBlock: true,
		},
		{
			name:      "hosts entry blocks request without fetch metadata",
			rules:     []string{`0.0.0.0 example.com`},
			url:       "https://example.com/",
			wantBlock: true,
		},
		{
			name:      "document exception does not cancel hosts entry on script",
			rules:     []string{`0.0.0.0 example.com`, `@@||example.com^$document`},
			url:       "https://example.com/script.js",
			headers:   http.Header{"Sec-Fetch-Dest": []string{"script"}},
			wantBlock: true,
		},
		{
			name:      "negated types block other types",
			rules:     negatedRules,
			url:       "https://example.com/adserver.js",
			headers:   http.Header{"Sec-Fetch-Dest": []string{"script"}},
			wantBlock: true,
		},
		{
			name:    "negated types do not block the first negated type",
			rules:   negatedRules,
			url:     "https://example.com/adserver.css",
			headers: http.Header{"Sec-Fetch-Dest": []string{"style"}},
		},
		{
			name:    "negated types do not block the second negated type",
			rules:   negatedRules,
			url:     "https://example.com/adserver.json",
			headers: http.Header{"Sec-Fetch-Dest": []string{"empty"}},
		},
		{
			name:  "negated type does not block a request without fetch metadata",
			rules: []string{`||example.com^$~image`},
			url:   "https://example.com/",
		},
		{
			name:  "negated ping does not block a request without fetch metadata",
			rules: []string{`||example.com^$~ping`},
			url:   "https://example.com/",
		},
		{
			name:  "negated websocket does not block a request without fetch metadata",
			rules: []string{`||example.com^$~websocket`},
			url:   "https://example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nr := New()
			for _, r := range tt.rules {
				if _, err := nr.ParseRule(r, nil); err != nil {
					t.Fatal(err)
				}
			}

			_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, tt.url, tt.headers))
			if shouldBlock != tt.wantBlock {
				t.Errorf("shouldBlock = %t, want %t", shouldBlock, tt.wantBlock)
			}
		})
	}
}

func TestUserActivatedNavigation(t *testing.T) {
	t.Parallel()

	clickedLinkHeaders := http.Header{
		"Sec-Fetch-Dest": []string{"document"},
		"Sec-Fetch-Mode": []string{"navigate"},
		"Sec-Fetch-User": []string{"?1"},
	}
	scriptNavigationHeaders := http.Header{
		"Sec-Fetch-Dest": []string{"document"},
		"Sec-Fetch-Mode": []string{"navigate"},
	}
	documentExceptionRules := []string{`||example.com^`, `@@||example.com^$document`}

	tests := []struct {
		name         string
		rules        []string
		url          string
		headers      http.Header
		wantBlock    bool
		wantRedirect string
	}{
		{
			name:         "untyped removeparam applies to clicked link",
			rules:        []string{`||example.com^$removeparam=utm_source`},
			url:          "https://example.com/?utm_source=news&id=1",
			headers:      clickedLinkHeaders,
			wantRedirect: "https://example.com/?id=1",
		},
		{
			name:    "untyped block rule does not block clicked link",
			rules:   []string{`||example.com^`},
			url:     "https://example.com/",
			headers: clickedLinkHeaders,
		},
		{
			name:      "untyped block rule blocks script-driven page load",
			rules:     []string{`||example.com^`},
			url:       "https://example.com/",
			headers:   scriptNavigationHeaders,
			wantBlock: true,
		},
		{
			name:      "later document rule blocks clicked link after skipped untyped rule",
			rules:     []string{`||example.com^`, `||example.com/page$document`},
			url:       "https://example.com/page",
			headers:   clickedLinkHeaders,
			wantBlock: true,
		},
		{
			name:    "document exception cancels untyped block rule on script-driven page load",
			rules:   documentExceptionRules,
			url:     "https://example.com/",
			headers: scriptNavigationHeaders,
		},
		{
			name:      "document exception does not cancel untyped block rule on script",
			rules:     documentExceptionRules,
			url:       "https://example.com/script.js",
			headers:   http.Header{"Sec-Fetch-Dest": []string{"script"}},
			wantBlock: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nr := New()
			for _, r := range tt.rules {
				if _, err := nr.ParseRule(r, nil); err != nil {
					t.Fatal(err)
				}
			}

			_, shouldBlock, redirectURL := nr.ModifyReq(newTestRequest(t, tt.url, tt.headers))
			if shouldBlock != tt.wantBlock {
				t.Errorf("shouldBlock = %t, want %t", shouldBlock, tt.wantBlock)
			}
			if redirectURL != tt.wantRedirect {
				t.Errorf("redirect URL = %q, want %q", redirectURL, tt.wantRedirect)
			}
		})
	}
}

func TestDocumentResponse(t *testing.T) {
	t.Parallel()

	nr := New()
	if _, err := nr.ParseRule(`||example.com^$document,removeheader=refresh`, nil); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name        string
		dest        string
		wantRefresh string
	}{
		{"removes header from page load", "document", ""},
		{"keeps header on script", "script", "5"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := &http.Response{Header: http.Header{
				"Content-Type": []string{"text/html"},
				"Refresh":      []string{"5"},
			}}
			req := newTestRequest(t, "https://example.com/", http.Header{"Sec-Fetch-Dest": []string{tt.dest}})
			if _, err := nr.ModifyRes(req, res); err != nil {
				t.Fatal(err)
			}
			if got := res.Header.Get("Refresh"); got != tt.wantRefresh {
				t.Errorf("Refresh = %q, want %q", got, tt.wantRefresh)
			}
		})
	}
}

func TestSharedModifiers(t *testing.T) {
	t.Parallel()

	navigationHeaders := http.Header{
		"Sec-Fetch-Dest": []string{"document"},
		"Sec-Fetch-User": []string{"?1"},
	}

	t.Run("second document rule blocks navigation", func(t *testing.T) {
		t.Parallel()

		nr := New()
		for _, r := range []string{`||a.example^$document`, `||b.example^$document`} {
			if _, err := nr.ParseRule(r, nil); err != nil {
				t.Fatal(err)
			}
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://b.example/", navigationHeaders))
		if !shouldBlock {
			t.Fatal("expected second document rule to block navigation")
		}
	})

	t.Run("second hosts rule blocks navigation", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`0.0.0.0 a.example b.example`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://b.example/", navigationHeaders))
		if !shouldBlock {
			t.Fatal("expected second hosts rule to block navigation")
		}
	})

	t.Run("second important rule resists non-important exception", func(t *testing.T) {
		t.Parallel()

		nr := New()
		for _, r := range []string{`||a.example^$image,important`, `||b.example^$image,important`, `@@||b.example^$image`} {
			if _, err := nr.ParseRule(r, nil); err != nil {
				t.Fatal(err)
			}
		}

		headers := http.Header{"Sec-Fetch-Dest": []string{"image"}}
		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://b.example/pixel.png", headers))
		if !shouldBlock {
			t.Fatal("expected second important rule to block despite non-important exception")
		}
	})

	t.Run("exception added after Compact applies", func(t *testing.T) {
		t.Parallel()

		nr := New()
		if _, err := nr.ParseRule(`||example.com^$document`, nil); err != nil {
			t.Fatal(err)
		}
		nr.Compact()
		if _, err := nr.ParseRule(`@@||example.com^$document`, nil); err != nil {
			t.Fatal(err)
		}

		_, shouldBlock, _ := nr.ModifyReq(newTestRequest(t, "https://example.com/", navigationHeaders))
		if shouldBlock {
			t.Fatal("expected exception added after Compact to cancel document rule")
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

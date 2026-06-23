package rulemodifiers

import (
	"net/http"
	"testing"
)

func TestContentTypeModifier_ShouldMatchReq(t *testing.T) {
	t.Parallel()

	t.Run("matches ping for ping modifier", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "ping"}
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"text/ping"}, "Ping-To": []string{"http://example.com"}},
		}

		if !m.ShouldMatchReq(req) {
			t.Fatal("expected to match ping")
		}
	})

	t.Run("matches websocket for websocket modifier", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "websocket"}
		req := &http.Request{
			Header: http.Header{"Sec-Fetch-Dest": []string{"websocket"}},
		}

		if !m.ShouldMatchReq(req) {
			t.Fatal("expected to match websocket")
		}
	})

	t.Run("empty Sec-Fetch-Dest with connection upgrade matches websocket", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "websocket"}
		req := &http.Request{
			Header: http.Header{"Connection": []string{"upgrade"}, "Upgrade": []string{"websocket"}},
		}

		if !m.ShouldMatchReq(req) {
			t.Fatal("expected websocket connection upgrade to match websocket")
		}
	})

	t.Run("does not match for connection upgrade with content-type != websocket", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "image"}
		req := &http.Request{
			Header: http.Header{"Connection": []string{"upgrade"}, "Upgrade": []string{"websocket"}},
		}

		if m.ShouldMatchReq(req) {
			t.Fatal("expected not to match wrong image modifier for connection upgrade")
		}
	})

	t.Run("inverted ping with no headers matches", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "ping", inverted: true}
		req := &http.Request{}

		if !m.ShouldMatchReq(req) {
			t.Fatal("expected inverted with no header to match")
		}
	})

	t.Run("inverted websocket with no headers matches", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "websocket", inverted: true}
		req := &http.Request{}

		if !m.ShouldMatchReq(req) {
			t.Fatal("expected inverted with no header to match")
		}
	})

	t.Run("empty Sec-Fetch-Dest and no connection upgrade does not match websocket", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "websocket"}
		req := &http.Request{}

		if m.ShouldMatchReq(req) {
			t.Fatal("expected not to match empty Sec-Fetch-Dest with no connection upgrade")
		}
	})

	t.Run("inverted does not match ping modifier", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "ping", inverted: true}
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"text/ping"}, "Ping-To": []string{"http://example.com"}},
		}

		if m.ShouldMatchReq(req) {
			t.Fatal("expected inverted not to match ping")
		}
	})
	t.Run("inverted does not match websocket modifier", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "websocket", inverted: true}
		req := &http.Request{
			Header: http.Header{"Sec-Fetch-Dest": []string{"websocket"}},
		}

		if m.ShouldMatchReq(req) {
			t.Fatal("expected inverted not to match websocket")
		}
	})

	t.Run("inverted with empty Sec-Fetch-Dest does not match", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "websocket", inverted: true}
		req := &http.Request{
			Header: http.Header{"Connection": []string{"upgrade"}, "Upgrade": []string{"websocket"}},
		}

		if m.ShouldMatchReq(req) {
			t.Fatal("expected inverted with empty Sec-Fetch-Dest not to match")
		}
	})

	t.Run("ping precedence over xmlhttprequest", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "ping"}
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"text/ping"}, "Ping-To": []string{"http://example.com"}, "Sec-Fetch-Dest": []string{"empty"}},
		}

		if !m.ShouldMatchReq(req) {
			t.Fatal("expected to match ping even with conflicting Sec-Fetch-Dest")
		}
	})

	t.Run("ping not matched as other", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "other"}
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"text/ping"}, "Ping-To": []string{"http://example.com"}},
		}

		if m.ShouldMatchReq(req) {
			t.Fatal("expected ping not to match other")
		}
	})

	t.Run("ping with charset matches", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "ping"}
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"text/ping; charset=utf-8"}, "Ping-To": []string{"http://example.com"}},
		}
		if !m.ShouldMatchReq(req) {
			t.Fatal("expected ping with charset and ping headers to match")
		}
	})
}

func TestContentTypeModifier_ShouldMatchRes(t *testing.T) {
	t.Parallel()

	t.Run("matches image/jpeg for image modifier", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "image"}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"image/jpeg"}},
		}

		if !m.ShouldMatchRes(res) {
			t.Fatal("expected to match image/jpeg")
		}
	})

	t.Run("does not match text/css for image modifier", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "image"}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"text/css"}},
		}

		if m.ShouldMatchRes(res) {
			t.Fatal("expected not to match text/css")
		}
	})

	t.Run("return false for empty Content-Type", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "image"}
		res := &http.Response{Header: http.Header{}}
		if m.ShouldMatchRes(res) {
			t.Fatal("expected false for empty Content-Type")
		}
	})

	t.Run("inverted does not match image/jpeg", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "image", inverted: true}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"image/jpeg"}},
		}

		if m.ShouldMatchRes(res) {
			t.Fatal("expected inverted not to match image/jpeg")
		}
	})

	t.Run("matches text/css for stylesheet modifier", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "stylesheet"}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"text/css"}},
		}

		if !m.ShouldMatchRes(res) {
			t.Fatal("expected to match text/css for stylesheet")
		}
	})

	t.Run("matches image/jpeg; with parameters", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "image"}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"image/jpeg; charset=utf-8"}},
		}

		if !m.ShouldMatchRes(res) {
			t.Fatal("expected to match image/jpeg; charset=utf-8 for image")
		}
	})

	t.Run("matches mixed-case mime type", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "script"}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"Application/JavaScript"}},
		}

		if !m.ShouldMatchRes(res) {
			t.Fatal("expected to match Application/JavaScript for script")
		}
	})

	t.Run("other matches unknown content type", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "other"}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"application/weird"}},
		}

		if !m.ShouldMatchRes(res) {
			t.Fatal("expected other to match unknown content type")
		}
	})

	t.Run("inverted other matches known content type", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "other", inverted: true}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"image/jpeg"}},
		}

		if !m.ShouldMatchRes(res) {
			t.Fatal("expected inverted other to match known content type")
		}
	})
}

package rulemodifiers

import (
	"net/http"
	"testing"
)

func TestContentTypeModifier(t *testing.T) {
	t.Parallel()

	t.Run("parse valid modifier without inversion", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		if err := m.Parse("stylesheet"); err != nil {
			t.Fatalf("ContentTypeModifier.Parse(\"stylesheet\") = %v, want nil", err)
		}
		if m.contentType != "stylesheet" {
			t.Errorf("m.contentType = %q, want %q", m.contentType, "stylesheet")
		}
		if m.inverted {
			t.Error("m.inverted = true, want false")
		}
	})

	t.Run("parse valid modifier with inversion", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		if err := m.Parse("~stylesheet"); err != nil {
			t.Fatalf("ContentTypeModifier.Parse(\"~stylesheet\") = %v, want nil", err)
		}
		if m.contentType != "stylesheet" {
			t.Errorf("m.contentType = %q, want %q", m.contentType, "stylesheet")
		}
		if !m.inverted {
			t.Error("m.inverted = false, want true")
		}
	})

	t.Run("parse alias css to stylesheet", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		if err := m.Parse("css"); err != nil {
			t.Fatalf("ContentTypeModifier.Parse(\"css\") = %v, want nil", err)
		}
		if m.contentType != "stylesheet" {
			t.Errorf("m.contentType = %q, want %q", m.contentType, "stylesheet")
		}
	})

	t.Run("ShouldMatchReq matches based on Sec-Fetch-Dest", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("script")

		req := &http.Request{
			Header: http.Header{
				"Sec-Fetch-Dest": []string{"script"},
			},
		}
		if !m.ShouldMatchReq(req) {
			t.Error("ContentTypeModifier.ShouldMatchReq(script) = false, want true")
		}

		req.Header.Set("Sec-Fetch-Dest", "image")
		if m.ShouldMatchReq(req) {
			t.Error("ContentTypeModifier.ShouldMatchReq(image) = true, want false")
		}
	})

	t.Run("ShouldMatchReq returns false without Sec-Fetch-Dest", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("script")

		req := &http.Request{Header: http.Header{}}
		if m.ShouldMatchReq(req) {
			t.Error("ContentTypeModifier.ShouldMatchReq(no header) = true, want false")
		}
	})

	// --- Response matching tests ---

	t.Run("ShouldMatchRes matches stylesheet", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("stylesheet")

		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"text/css"},
			},
		}
		if !m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier.ShouldMatchRes(text/css) = false, want true")
		}

		res.Header.Set("Content-Type", "text/html")
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier.ShouldMatchRes(text/html) = true, want false")
		}
	})

	t.Run("ShouldMatchRes matches script", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("script")

		tests := []struct {
			mime string
			want bool
		}{
			{"text/javascript", true},
			{"application/javascript", true},
			{"application/ecmascript", true},
			{"text/css", false},
			{"text/html", false},
		}

		for _, tt := range tests {
			res := &http.Response{
				Header: http.Header{
					"Content-Type": []string{tt.mime},
				},
			}
			got := m.ShouldMatchRes(res)
			if got != tt.want {
				t.Errorf("ContentTypeModifier(script).ShouldMatchRes(%q) = %t, want %t", tt.mime, got, tt.want)
			}
		}
	})

	t.Run("ShouldMatchRes matches image", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("image")

		for _, mime := range []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml"} {
			res := &http.Response{
				Header: http.Header{
					"Content-Type": []string{mime},
				},
			}
			if !m.ShouldMatchRes(res) {
				t.Errorf("ContentTypeModifier(image).ShouldMatchRes(%q) = false, want true", mime)
			}
		}

		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"text/plain"},
			},
		}
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(image).ShouldMatchRes(text/plain) = true, want false")
		}
	})

	t.Run("ShouldMatchRes matches media", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("media")

		for _, mime := range []string{"audio/mpeg", "audio/ogg", "video/mp4", "video/webm"} {
			res := &http.Response{
				Header: http.Header{
					"Content-Type": []string{mime},
				},
			}
			if !m.ShouldMatchRes(res) {
				t.Errorf("ContentTypeModifier(media).ShouldMatchRes(%q) = false, want true", mime)
			}
		}

		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"text/plain"},
			},
		}
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(media).ShouldMatchRes(text/plain) = true, want false")
		}
	})

	t.Run("ShouldMatchRes matches font", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("font")

		for _, mime := range []string{"font/woff2", "font/ttf", "font/otf"} {
			res := &http.Response{
				Header: http.Header{
					"Content-Type": []string{mime},
				},
			}
			if !m.ShouldMatchRes(res) {
				t.Errorf("ContentTypeModifier(font).ShouldMatchRes(%q) = false, want true", mime)
			}
		}
	})

	t.Run("ShouldMatchRes matches subdocument", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("subdocument")

		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"text/html"},
			},
		}
		if !m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(subdocument).ShouldMatchRes(text/html) = false, want true")
		}

		res.Header.Set("Content-Type", "text/plain")
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(subdocument).ShouldMatchRes(text/plain) = true, want false")
		}
	})

	t.Run("ShouldMatchRes matches object", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("object")

		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/x-shockwave-flash"},
			},
		}
		if !m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(object).ShouldMatchRes(flash) = false, want true")
		}
	})

	t.Run("ShouldMatchRes respects parameters in Content-Type", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("stylesheet")

		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"text/css; charset=utf-8"},
			},
		}
		if !m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(stylesheet).ShouldMatchRes(text/css; charset=utf-8) = false, want true")
		}
	})

	t.Run("ShouldMatchRes returns false on empty Content-Type", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("image")

		res := &http.Response{Header: http.Header{}}
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(image).ShouldMatchRes(no Content-Type) = true, want false")
		}
	})

	t.Run("ShouldMatchRes handles inverted modifier", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("~image")

		// Not an image → should match (inverted)
		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"text/plain"},
			},
		}
		if !m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(~image).ShouldMatchRes(text/plain) = false, want true")
		}

		// Is an image → should NOT match (inverted)
		res.Header.Set("Content-Type", "image/png")
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(~image).ShouldMatchRes(image/png) = true, want false")
		}
	})

	t.Run("ShouldMatchRes handles other modifier", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("other")

		// Unknown MIME type → should match "other"
		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/octet-stream"},
			},
		}
		if !m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(other).ShouldMatchRes(application/octet-stream) = false, want true")
		}

		// Known MIME type → should NOT match "other"
		res.Header.Set("Content-Type", "text/css")
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(other).ShouldMatchRes(text/css) = true, want false")
		}
	})

	t.Run("ShouldMatchRes xmlhttprequest is request-only", func(t *testing.T) {
		t.Parallel()
		m := ContentTypeModifier{}
		m.Parse("xmlhttprequest")

		res := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		}
		if m.ShouldMatchRes(res) {
			t.Error("ContentTypeModifier(xmlhttprequest).ShouldMatchRes() = true, want false")
		}
	})

	t.Run("Cancels identical modifiers", func(t *testing.T) {
		t.Parallel()
		a := ContentTypeModifier{contentType: "stylesheet", inverted: false}
		b := ContentTypeModifier{contentType: "stylesheet", inverted: false}
		if !a.Cancels(&b) {
			t.Error("ContentTypeModifier.Cancels(identical) = false, want true")
		}
	})

	t.Run("Cancels different modifiers", func(t *testing.T) {
		t.Parallel()
		a := ContentTypeModifier{contentType: "stylesheet", inverted: false}
		b := ContentTypeModifier{contentType: "script", inverted: false}
		if a.Cancels(&b) {
			t.Error("ContentTypeModifier.Cancels(different) = true, want false")
		}
	})
}

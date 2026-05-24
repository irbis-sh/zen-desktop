package rulemodifiers

import (
	"net/http"
	"testing"
)

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

	t.Run("other matches unknown content type", func(t *testing.T) {
		t.Parallel()

		m := &ContentTypeModifier{contentType: "other"}
		res := &http.Response{
			Header: http.Header{"Content-Type": []string{"application/weird"}},
		}

		if !m.ShouldMatchRes(res) {
			t.Fatal("expected other to match unkwnow content type")
		}
	})
}

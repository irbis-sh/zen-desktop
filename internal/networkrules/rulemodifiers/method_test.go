package rulemodifiers

import (
	"net/http"
	"net/url"
	"testing"
)

func TestMethodModifier(t *testing.T) {
	t.Parallel()

	t.Run("single method", func(t *testing.T) {
		t.Parallel()

		m := MethodModifier{}
		m.Parse("method=GET")
		req := http.Request{
			Method: "GET",
		}
		if !m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=GET should match a GET request")
		}
	})

	t.Run("single inverted method", func(t *testing.T) {
		t.Parallel()

		m := MethodModifier{}
		m.Parse("method=~GET")
		req := http.Request{
			Method: "GET",
		}
		if m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=~GET should not match a GET request")
		}
	})

	t.Run("lowercase method", func(t *testing.T) {
		t.Parallel()

		m := MethodModifier{}
		m.Parse("method=get")
		req := http.Request{
			Method: "GET",
		}
		if !m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=get should match a GET request")
		}
	})

	t.Run("multiple methods", func(t *testing.T) {
		t.Parallel()

		m := MethodModifier{}
		m.Parse("method=GET|POST")

		req := http.Request{
			Method: "GET",
		}
		if !m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=GET|POST should match a GET request")
		}

		req.Method = "POST"
		if !m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=GET|POST should match a POST request")
		}

		req.Method = "HEAD"
		if m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=GET|POST should not match a HEAD request")
		}
	})

	t.Run("multiple inverted methods", func(t *testing.T) {
		t.Parallel()

		m := MethodModifier{}
		m.Parse("method=~GET|~POST")

		req := http.Request{
			Method: "GET",
		}
		if m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=~GET|~POST should not match a GET request")
		}

		req.Method = "POST"
		if m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=~GET|~POST should not match a POST request")
		}

		req.Method = "HEAD"
		if !m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=~GET|~POST should match a HEAD request")
		}

		req.Method = "PUT"
		if !m.ShouldMatchReq(Request{Request: &req}) {
			t.Error("method=~GET|~POST should match a PUT request")
		}
	})

	t.Run("mixed inverted and non-inverted methods", func(t *testing.T) {
		t.Parallel()

		m := MethodModifier{}
		if err := m.Parse("method=GET|~POST"); err == nil {
			t.Error("method=GET|~POST should return an error")
		}

		m = MethodModifier{}
		if err := m.Parse("method=~GET|POST"); err == nil {
			t.Error("method=~GET|POST should return an error")
		}
	})
}

func TestMethodModifierShouldMatchRes(t *testing.T) {
	t.Parallel()

	m := MethodModifier{}
	if err := m.Parse("method=GET"); err != nil {
		t.Fatal(err)
	}
	res := &http.Response{Request: &http.Request{Method: "GET", URL: &url.URL{Scheme: "https", Host: "example.com"}}}
	if !m.ShouldMatchRes(res) {
		t.Error("MethodModifier.ShouldMatchRes(res) = false, want true")
	}
	res.Request.Method = "POST"
	if m.ShouldMatchRes(res) {
		t.Error("MethodModifier.ShouldMatchRes(res) = true, want false")
	}

	inv := MethodModifier{}
	if err := inv.Parse("method=~GET"); err != nil {
		t.Fatal(err)
	}
	res.Request.Method = "GET"
	if inv.ShouldMatchRes(res) {
		t.Error("inverted MethodModifier.ShouldMatchRes(res) = true, want false")
	}
	res.Request.Method = "POST"
	if !inv.ShouldMatchRes(res) {
		t.Error("inverted MethodModifier.ShouldMatchRes(res) = false, want true")
	}

}

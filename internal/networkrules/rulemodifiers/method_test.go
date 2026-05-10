package rulemodifiers

import (
	"net/http"
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
		if !m.ShouldMatchReq(&req) {
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
		if m.ShouldMatchReq(&req) {
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
		if !m.ShouldMatchReq(&req) {
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
		if !m.ShouldMatchReq(&req) {
			t.Error("method=GET|POST should match a GET request")
		}

		req.Method = "POST"
		if !m.ShouldMatchReq(&req) {
			t.Error("method=GET|POST should match a POST request")
		}

		req.Method = "HEAD"
		if m.ShouldMatchReq(&req) {
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
		if m.ShouldMatchReq(&req) {
			t.Error("method=~GET|~POST should not match a GET request")
		}

		req.Method = "POST"
		if m.ShouldMatchReq(&req) {
			t.Error("method=~GET|~POST should not match a POST request")
		}

		req.Method = "HEAD"
		if !m.ShouldMatchReq(&req) {
			t.Error("method=~GET|~POST should match a HEAD request")
		}

		req.Method = "PUT"
		if !m.ShouldMatchReq(&req) {
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

	t.Run("cancels", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			a    MethodModifier
			b    MethodModifier
			want bool
		}{
			{
				"Should cancel - identical modifiers",
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
					},
					inverted: true,
				},
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
					},
					inverted: true,
				},
				true,
			},
			{
				"Should cancel - identical modifiers",
				MethodModifier{},
				MethodModifier{},
				true,
			},
			{
				"Should cancel - identical methods but different order",
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "POST"},
						{method: "GET"},
					},
					inverted: true,
				},
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
					},
					inverted: true,
				},
				true,
			},
			{
				"Should not cancel - different method entries",
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
					},
					inverted: true,
				},
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "PUT"},
						{method: "DELETE"},
					},
					inverted: true,
				},
				false,
			},
			{
				"Should not cancel - different inverted flag",
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
					},
					inverted: true,
				},
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
					},
					inverted: false,
				},
				false,
			},
			{
				"Should not cancel - one has extra method",
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
					},
					inverted: true,
				},
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
						{method: "POST"},
						{method: "DELETE"},
					},
					inverted: true,
				},
				false,
			},
			{
				"Should not cancel - one is empty",
				MethodModifier{
					entries:  []methodModifierEntry{},
					inverted: true,
				},
				MethodModifier{
					entries: []methodModifierEntry{
						{method: "GET"},
					},
					inverted: true,
				},
				false,
			},
			{
				"Should cancel - both are empty",
				MethodModifier{
					entries:  []methodModifierEntry{},
					inverted: true,
				},
				MethodModifier{
					entries:  []methodModifierEntry{},
					inverted: true,
				},
				true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				result := tt.a.Cancels(&tt.b)
				if result != tt.want {
					t.Errorf("MethodModifier.Cancels() = %t, want %t", result, tt.want)
				}
			})
		}
	})
}

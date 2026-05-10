package rulemodifiers

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestJsonPruneModifier(t *testing.T) {
	t.Parallel()

	t.Run("ModifyRes", func(t *testing.T) {
		t.Parallel()

		mustParse := func(mod string) *JSONPruneModifier {
			t.Helper()
			var m JSONPruneModifier
			if err := m.Parse(mod); err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			return &m
		}

		tests := []struct {
			name     string
			modifier string
			input    string
			want     string
			modified bool
		}{
			{
				name:     "remove top-level field with escaped $",
				modifier: `jsonprune=$.removeMe`,
				input:    `{"removeMe":1,"keep":2}`,
				want:     `{"keep":2}`,
				modified: true,
			},
			{
				name:     "remove nested field",
				modifier: `jsonprune=$.a.b`,
				input:    `{"a":{"b":1,"c":2}}`,
				want:     `{"a":{"c":2}}`,
				modified: true,
			},
			{
				name:     "remove from array",
				modifier: `jsonprune=$.items[*].ad`,
				input:    `{"items":[{"id":1,"ad":true},{"id":2,"ad":false}]}`,
				want:     `{"items":[{"id":1},{"id":2}]}`,
				modified: true,
			},
			{
				name:     "non-matching path is no-op",
				modifier: `jsonprune=$.notThere`,
				input:    `{"some":"thing"}`,
				want:     `{"some":"thing"}`,
				modified: false,
			},
			{
				name:     "escaped comma inside path",
				modifier: `jsonprune=$..['adSlots','adPlacements']`,
				input:    `{"adSlots":[1,2],"adPlacements":[3,4],"keep":true}`,
				want:     `{"keep":true}`,
				modified: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mod := mustParse(tt.modifier)

				res := &http.Response{
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body:          io.NopCloser(strings.NewReader(tt.input)),
					ContentLength: int64(len(tt.input)),
				}

				modified, err := mod.ModifyRes(res)
				if err != nil {
					t.Errorf("error modifying res: %v", err)
				}
				if modified != tt.modified {
					t.Errorf("ModifyRes() = %v, want %v", modified, tt.modified)
				}

				body, _ := io.ReadAll(res.Body)
				res.Body.Close()

				if string(body) != tt.want {
					t.Errorf("response body = %q, want %q", string(body), tt.want)
				}
			})
		}
	})

	t.Run("Cancels", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			a        *JSONPruneModifier
			b        Modifier
			expected bool
		}{
			{
				name:     "same path cancels",
				a:        mustParseJSONPrune(t, `jsonprune=$.a.b`),
				b:        mustParseJSONPrune(t, `jsonprune=$.a.b`),
				expected: true,
			},
			{
				name:     "different paths do not cancel",
				a:        mustParseJSONPrune(t, `jsonprune=$.a`),
				b:        mustParseJSONPrune(t, `jsonprune=$.b`),
				expected: false,
			},
			{
				name:     "different types do not cancel",
				a:        mustParseJSONPrune(t, `jsonprune=$.x`),
				b:        &RemoveHeaderModifier{HeaderName: "X-Test"},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := tt.a.Cancels(tt.b)
				if result != tt.expected {
					t.Errorf("Cancels() = %v, want %v", result, tt.expected)
				}
			})
		}
	})
}

func mustParseJSONPrune(t *testing.T, modifier string) *JSONPruneModifier {
	t.Helper()
	var m JSONPruneModifier
	if err := m.Parse(modifier); err != nil {
		t.Fatalf("failed to parse modifier %q: %v", modifier, err)
	}
	return &m
}

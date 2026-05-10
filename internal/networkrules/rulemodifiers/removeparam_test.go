package rulemodifiers

import (
	"net/url"
	"regexp"
	"testing"
)

func TestRemoveParamModifier(t *testing.T) {
	t.Parallel()

	t.Run("ModifyReq", func(t *testing.T) {
		t.Parallel()

		mustParse := func(modifier string) RemoveParamModifier {
			t.Helper()

			var rm RemoveParamModifier
			if err := rm.Parse(modifier); err != nil {
				t.Fatalf("Failed to parse modifier: %v", err)
			}
			return rm
		}

		tests := []struct {
			name     string
			modifier RemoveParamModifier
			url      string
			want     string
			modified bool
		}{
			{
				name:     "generic removes all params",
				modifier: mustParse("removeparam"),
				url:      "https://example.com/?id=123&name=test",
				want:     "https://example.com/",
				modified: true,
			},
			{
				name:     "exact removes the specified param",
				modifier: mustParse("removeparam=id"),
				url:      "https://example.com/?id=123&known=1&name=test",
				want:     "https://example.com/?known=1&name=test",
				modified: true,
			},
			{
				name:     "exact leaves the URL unchanged if the param is not found",
				modifier: mustParse("removeparam=unknown"),
				url:      "https://example.com/?id=123&name=test&known=1",
				want:     "https://example.com/?id=123&name=test&known=1",
				modified: false,
			},
			{
				name:     "exact leaves the URL unchanged if the URL has no query params",
				modifier: mustParse("removeparam=id"),
				url:      "https://example.com:443/",
				want:     "https://example.com:443/",
				modified: false,
			},
			{
				name:     "exact inverse removes all params except the specified one",
				modifier: mustParse("removeparam=~id"),
				url:      "https://example.com/?id=123&name=test&known=1",
				want:     "https://example.com/?id=123",
				modified: true,
			},
			{
				name:     "regexp removes matching params",
				modifier: mustParse("removeparam=/id=\\d+/"),
				url:      "https://example.com/?id=123&name=test",
				want:     "https://example.com/?name=test",
				modified: true,
			},
			{
				name:     "regexp leaves the URL unchanged if the param is not found",
				modifier: mustParse("removeparam=/id=[a-z]+/"),
				url:      "https://example.com/?id=123&name=test",
				want:     "https://example.com/?id=123&name=test",
				modified: false,
			},
			{
				name:     "regexp only removes matching params for values with the same key",
				modifier: mustParse("removeparam=/id=\\d+/"),
				url:      "https://example.com/?id=test0&id=1&id=2&id=3&id=test1&id=test2",
				want:     "https://example.com/?id=test0&id=test1&id=test2",
				modified: true,
			},
			{
				name:     "inverse regexp removes non-matching params",
				modifier: mustParse("removeparam=~/id=\\d+/"),
				url:      "https://example.com/?id=123&name=test&known=1",
				want:     "https://example.com/?id=123",
				modified: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				u, err := url.Parse(tt.url)
				if err != nil {
					t.Fatalf("parse URL %q: %v", tt.url, err)
				}

				q := u.Query()

				modified := tt.modifier.ModifyQuery(q)
				if modified != tt.modified {
					t.Errorf("ModifyReq() modified = %v, want %v", modified, tt.modified)
				}

				if modified {
					u.RawQuery = q.Encode()
				}

				got := u.String()
				if got != tt.want {
					t.Errorf("ModifyReq() got URL = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("cancels", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			a        RemoveParamModifier
			b        RemoveParamModifier
			expected bool
		}{
			{
				"identical modifiers - should cancel",
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				true,
			},
			{
				"empty modifiers - should cancel",
				RemoveParamModifier{},
				RemoveParamModifier{},
				true,
			},
			{
				"modifiers with different \"param\" - should not cancel",
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "user",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				false,
			},
			{
				"modifiers with different \"kind\" - should not cancel",
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				RemoveParamModifier{
					kind:   removeparamKindExact,
					param:  "id",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				false,
			},
			{
				"modifiers with different \"regexp\" - should not cancel",
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: regexp.MustCompile("^[a-zA-Z]+$"),
				},
				false,
			},
			{
				"modifiers with nil regexes - should cancel",
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: nil,
				},
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: nil,
				},
				true,
			},
			{
				"modifier with nil regex should not cancel with non-nil regex",
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: nil,
				},
				RemoveParamModifier{
					kind:   removeparamKindRegexp,
					param:  "id",
					regexp: regexp.MustCompile(`^\d+$`),
				},
				false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				result := tt.a.Cancels(&tt.b)
				if result != tt.expected {
					t.Errorf("RemoveParamModifier.Cancels() = %t, want %t", result, tt.expected)
				}
			})
		}
	})
}

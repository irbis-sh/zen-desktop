package rule

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/contenttype"
)

func TestParseModifiers(t *testing.T) {
	t.Parallel()

	t.Run("empty modifier list is a no-op", func(t *testing.T) {
		t.Parallel()

		var r Rule
		if err := r.ParseModifiers(nil); err != nil {
			t.Fatalf("ParseModifiers(nil) = %v, want nil", err)
		}

		assertRuleBuckets(t, &r, false, false, 0, nil, nil, nil)
	})

	t.Run("important modifier sets important flag only", func(t *testing.T) {
		t.Parallel()

		var r Rule
		if err := r.ParseModifiers([]string{"important"}); err != nil {
			t.Fatalf("ParseModifiers(%q) = %v, want nil", "important", err)
		}

		assertRuleBuckets(t, &r, true, false, 0, nil, nil, nil)
	})

	t.Run("noop modifiers do not allocate a modifier block", func(t *testing.T) {
		t.Parallel()

		var r Rule
		if err := r.ParseModifiers([]string{"_", "___"}); err != nil {
			t.Fatalf("ParseModifiers() = %v, want nil", err)
		}

		if r.mods != nil {
			t.Errorf("mods = %+v, want nil", r.mods)
		}
	})

	t.Run("document modifiers set document type and block navigations", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{"document", "doc"} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				if err := r.ParseModifiers([]string{modifier}); err != nil {
					t.Fatalf("ParseModifiers(%q) = %v, want nil", modifier, err)
				}

				assertRuleBuckets(t, &r, false, true, contenttype.Mask(contenttype.Document), nil, nil, nil)
			})
		}
	})

	t.Run("classifies modifiers", func(t *testing.T) {
		t.Parallel()

		var r Rule
		err := r.ParseModifiers([]string{
			"domain=example.com",
			"method=get",
			"third-party",
			"header=set-cookie",
			"xmlhttprequest",
			"xhr",
			"font",
			"subdocument",
			"image",
			"object",
			"script",
			"stylesheet",
			"media",
			"other",
			"removeparam",
			"removeparam=utm_source",
			"removeheader=X-Test",
			"remove-js-constant=window.ad",
			"scramblejs=tracker",
			"jsonprune=$.ads",
		})
		if err != nil {
			t.Fatalf("ParseModifiers() = %v, want nil", err)
		}

		assertRuleBuckets(t, &r, false, false,
			contenttype.Mask(contenttype.XMLHTTPRequest|contenttype.Font|contenttype.Subdocument|contenttype.Image|
				contenttype.Object|contenttype.Script|contenttype.Stylesheet|contenttype.Media|contenttype.Other),
			[]string{
				"*rulemodifiers.DomainModifier",
				"*rulemodifiers.MethodModifier",
				"*rulemodifiers.ThirdPartyModifier",
				"*rulemodifiers.HeaderModifier",
			},
			[]string{
				"*rulemodifiers.RemoveParamModifier",
				"*rulemodifiers.RemoveParamModifier",
			},
			[]string{
				"*rulemodifiers.RemoveHeaderModifier",
				"*removejsconstant.Modifier",
				"*rulemodifiers.ScrambleJSModifier",
				"*rulemodifiers.JSONPruneModifier",
			},
		)
	})

	t.Run("negated content types exclude each type", func(t *testing.T) {
		t.Parallel()

		r := newTestRule(t, "~stylesheet", "~xmlhttprequest")

		want := contenttype.All &^ contenttype.Mask(contenttype.Stylesheet|contenttype.XMLHTTPRequest)
		if got := r.types(); got != want {
			t.Errorf("types = %#x, want %#x", got, want)
		}
	})

	t.Run("negated document clears document type and does not block navigations", func(t *testing.T) {
		t.Parallel()

		r := newTestRule(t, "~document")

		assertRuleBuckets(t, r, false, false, contenttype.All&^contenttype.Mask(contenttype.Document), nil, nil, nil)
	})

	t.Run("all matches any type and blocks navigations", func(t *testing.T) {
		t.Parallel()

		r := newTestRule(t, "all")

		assertRuleBuckets(t, r, false, true, 0, nil, nil, nil)
	})

	t.Run("negated content types narrow positive ones", func(t *testing.T) {
		t.Parallel()

		r := newTestRule(t, "script", "image", "~image")

		if got, want := r.types(), contenttype.Mask(contenttype.Script); got != want {
			t.Errorf("types = %#x, want %#x", got, want)
		}
	})

	t.Run("all covers every type next to content types", func(t *testing.T) {
		t.Parallel()

		if got := newTestRule(t, "all", "script").types(); got != 0 {
			t.Errorf("types = %#x, want 0", got)
		}

		want := contenttype.All &^ contenttype.Mask(contenttype.Image)
		if got := newTestRule(t, "all", "script", "~image").types(); got != want {
			t.Errorf("types = %#x, want %#x", got, want)
		}
	})

	t.Run("rejects content types that cancel out", func(t *testing.T) {
		t.Parallel()

		var r Rule
		if err := r.ParseModifiers([]string{"script", "~script"}); err == nil {
			t.Fatal("ParseModifiers() = nil, want error")
		}
	})

	t.Run("rejects negated non-type flags", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{"~all", "~important", "~removeparam", "~removeparam=utm_source", "~domain=example.com"} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				err := r.ParseModifiers([]string{modifier})
				if err == nil || !strings.Contains(err.Error(), "unknown modifier") {
					t.Fatalf("ParseModifiers(%q) = %v, want unknown modifier error", modifier, err)
				}
			})
		}
	})

	t.Run("accepts negated third-party", func(t *testing.T) {
		t.Parallel()

		r := newTestRule(t, "~third-party")
		if got := typeNames(r.conditions()); !reflect.DeepEqual(got, []string{"*rulemodifiers.ThirdPartyModifier"}) {
			t.Errorf("condition modifiers = %#v, want ThirdPartyModifier", got)
		}
	})

	t.Run("noop modifiers are ignored", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{"_", "__", "___", "____"} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				if err := r.ParseModifiers([]string{modifier}); err != nil {
					t.Fatalf("ParseModifiers(%q) = %v, want nil", modifier, err)
				}

				assertRuleBuckets(t, &r, false, false, 0, nil, nil, nil)
			})
		}
	})

	t.Run("noop modifiers do not affect other modifiers", func(t *testing.T) {
		t.Parallel()

		var r Rule
		if err := r.ParseModifiers([]string{"script", "_", "domain=example.com", "__"}); err != nil {
			t.Fatalf("ParseModifiers() = %v, want nil", err)
		}

		assertRuleBuckets(t, &r, false, false,
			contenttype.Mask(contenttype.Script),
			[]string{"*rulemodifiers.DomainModifier"},
			nil,
			nil,
		)
	})

	t.Run("rejects noop-like modifiers with non-underscore characters", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{
			"_abc",      // letters after underscore
			"abc_",      // letters before underscore
			"___=value", // noop with a value
			"_=_",       // underscore with a value
		} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				err := r.ParseModifiers([]string{modifier})
				if err == nil {
					t.Fatalf("ParseModifiers(%q) = nil, want unknown modifier error", modifier)
				}
				if !strings.Contains(err.Error(), "unknown modifier") {
					t.Fatalf("ParseModifiers(%q) error = %q, want unknown modifier", modifier, err)
				}
			})
		}
	})

	t.Run("rejects prefix collisions", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{
			"scriptlet",
			"domainish=example.com",
			"methodology=get",
			"removeparametric=id",
			"removeheaderish=X-Test",
			"documentary",
		} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				err := r.ParseModifiers([]string{modifier})
				if err == nil {
					t.Fatalf("ParseModifiers(%q) = nil, want unknown modifier error", modifier)
				}
				if !strings.Contains(err.Error(), "unknown modifier") {
					t.Fatalf("ParseModifiers(%q) error = %q, want unknown modifier", modifier, err)
				}
			})
		}
	})

	t.Run("rejects flag modifiers with values", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{
			"script=foo",
			"third-party=true",
			"document=1",
			"all=true",
		} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				err := r.ParseModifiers([]string{modifier})
				if err == nil {
					t.Fatalf("ParseModifiers(%q) = nil, want unknown modifier error", modifier)
				}
				if !strings.Contains(err.Error(), "unknown modifier") {
					t.Fatalf("ParseModifiers(%q) error = %q, want unknown modifier", modifier, err)
				}
			})
		}
	})
}

func TestShouldMatchReq(t *testing.T) {
	t.Parallel()

	r := newTestRule(t, "script", "domain=example.com")

	tests := []struct {
		name    string
		reqType contenttype.Type
		referer string
		want    bool
	}{
		{"matching type and domain", contenttype.Script, "https://example.com/", true},
		{"other type", contenttype.Image, "https://example.com/", false},
		{"other domain", contenttype.Script, "https://other.com/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := newTestRequest(map[string]string{"Referer": tt.referer})
			if got := r.ShouldMatchReq(req, tt.reqType); got != tt.want {
				t.Errorf("ShouldMatchReq() = %t, want %t", got, tt.want)
			}
		})
	}
}

func assertRuleBuckets(t *testing.T, r *Rule, wantImportant, wantBlocksNavigations bool, wantTypes contenttype.Mask, wantConditions, wantQuery, wantActions []string) {
	t.Helper()

	if r.Important() != wantImportant {
		t.Errorf("Important() = %v, want %v", r.Important(), wantImportant)
	}
	if r.BlocksNavigations() != wantBlocksNavigations {
		t.Errorf("BlocksNavigations() = %v, want %v", r.BlocksNavigations(), wantBlocksNavigations)
	}
	if got := r.types(); got != wantTypes {
		t.Errorf("types = %#x, want %#x", got, wantTypes)
	}
	if got := typeNames(r.conditions()); !reflect.DeepEqual(got, wantConditions) {
		t.Errorf("condition modifiers = %#v, want %#v", got, wantConditions)
	}
	if got := typeNames(r.QueryModifiers()); !reflect.DeepEqual(got, wantQuery) {
		t.Errorf("query modifiers = %#v, want %#v", got, wantQuery)
	}
	if got := typeNames(r.ActionModifiers()); !reflect.DeepEqual(got, wantActions) {
		t.Errorf("action modifiers = %#v, want %#v", got, wantActions)
	}
}

func typeNames[T any](items []T) []string {
	if len(items) == 0 {
		return nil
	}

	names := make([]string, len(items))
	for i, item := range items {
		names[i] = reflect.TypeOf(item).String()
	}
	return names
}

func newTestRule(t *testing.T, modifiers ...string) *Rule {
	t.Helper()

	var r Rule
	if err := r.ParseModifiers(modifiers); err != nil {
		t.Fatalf("ParseModifiers(%q) = %v, want nil", modifiers, err)
	}
	return &r
}

func newTestRequest(headers map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "https://target.example/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

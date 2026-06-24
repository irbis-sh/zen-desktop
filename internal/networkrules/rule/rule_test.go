package rule

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseModifiers(t *testing.T) {
	t.Parallel()

	t.Run("empty modifier list is a no-op", func(t *testing.T) {
		t.Parallel()

		var r Rule
		if err := r.ParseModifiers(nil); err != nil {
			t.Fatalf("ParseModifiers(nil) = %v, want nil", err)
		}

		assertRuleBuckets(t, &r, false, false, nil, nil, nil, nil)
	})

	t.Run("important modifier sets important flag only", func(t *testing.T) {
		t.Parallel()

		var r Rule
		if err := r.ParseModifiers([]string{"important"}); err != nil {
			t.Fatalf("ParseModifiers(%q) = %v, want nil", "important", err)
		}

		assertRuleBuckets(t, &r, true, false, nil, nil, nil, nil)
	})

	t.Run("document modifiers set document flag only", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{"document", "doc"} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				if err := r.ParseModifiers([]string{modifier}); err != nil {
					t.Fatalf("ParseModifiers(%q) = %v, want nil", modifier, err)
				}

				assertRuleBuckets(t, &r, false, true, nil, nil, nil, nil)
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
			"all",
		})
		if err != nil {
			t.Fatalf("ParseModifiers() = %v, want nil", err)
		}

		assertRuleBuckets(t, &r, false, false,
			[]string{
				"*rulemodifiers.DomainModifier",
				"*rulemodifiers.MethodModifier",
				"*rulemodifiers.ThirdPartyModifier",
				"*rulemodifiers.HeaderModifier",
			},
			[]string{
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
				"*rulemodifiers.ContentTypeModifier",
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

	t.Run("noop modifiers are ignored", func(t *testing.T) {
		t.Parallel()

		for _, modifier := range []string{"_", "__", "___", "____"} {
			t.Run(modifier, func(t *testing.T) {
				t.Parallel()

				var r Rule
				if err := r.ParseModifiers([]string{modifier}); err != nil {
					t.Fatalf("ParseModifiers(%q) = %v, want nil", modifier, err)
				}

				assertRuleBuckets(t, &r, false, false, nil, nil, nil, nil)
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
			[]string{"*rulemodifiers.DomainModifier"},
			[]string{"*rulemodifiers.ContentTypeModifier"},
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

func assertRuleBuckets(t *testing.T, r *Rule, wantImportant, wantDocument bool, wantAnd, wantOr, wantQuery, wantActions []string) {
	t.Helper()

	if r.Important != wantImportant {
		t.Errorf("Important = %v, want %v", r.Important, wantImportant)
	}
	if r.Document != wantDocument {
		t.Errorf("Document = %v, want %v", r.Document, wantDocument)
	}
	if got := typeNames(r.ConditionModifiers.And); !reflect.DeepEqual(got, wantAnd) {
		t.Errorf("AND modifiers = %#v, want %#v", got, wantAnd)
	}
	if got := typeNames(r.ConditionModifiers.Or); !reflect.DeepEqual(got, wantOr) {
		t.Errorf("OR modifiers = %#v, want %#v", got, wantOr)
	}
	if got := typeNames(r.QueryModifiers); !reflect.DeepEqual(got, wantQuery) {
		t.Errorf("query modifiers = %#v, want %#v", got, wantQuery)
	}
	if got := typeNames(r.ActionModifiers); !reflect.DeepEqual(got, wantActions) {
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

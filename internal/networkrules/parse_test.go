package networkrules

import (
	"fmt"
	"reflect"
	"testing"
)

func TestParseRuleParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rawRule       string
		wantPattern   string
		wantModifiers []string
	}{
		{
			name:          "normal rule with modifiers",
			rawRule:       "||example.com^$script",
			wantPattern:   "||example.com^",
			wantModifiers: []string{"script"},
		},
		{
			name:          "regexp rule with dollar anchor",
			rawRule:       `/foo$/$script`,
			wantPattern:   `/foo$/`,
			wantModifiers: []string{"script"},
		},
		{
			name:          "empty pattern with modifiers",
			rawRule:       "$script",
			wantPattern:   "",
			wantModifiers: []string{"script"},
		},
		{
			name:          "regexp rule with escaped slash",
			rawRule:       `/foo\/bar/$script`,
			wantPattern:   `/foo\/bar/`,
			wantModifiers: []string{"script"},
		},
		{
			name:          "regexp rule with unescaped slash",
			rawRule:       `/example.com/[0-9a-z]*.php/$domain=example.com`,
			wantPattern:   `/example.com/[0-9a-z]*.php/`,
			wantModifiers: []string{"domain=example.com"},
		},
		{
			name:          "regexp rule with regexp modifier",
			rawRule:       `/^https:\/\/d[0-9a-z]{12,13}\.cloudfront\.net\/loader\.min\.js$/$script,third-party,header=etag:/^W\/"[0-9a-f]{32}"$/`,
			wantPattern:   `/^https:\/\/d[0-9a-z]{12,13}\.cloudfront\.net\/loader\.min\.js$/`,
			wantModifiers: []string{"script", "third-party", `header=etag:/^W\/"[0-9a-f]{32}"$/`},
		},
		{
			name:          "slash-prefixed normal rule",
			rawRule:       `/path/like/filter$script`,
			wantPattern:   `/path/like/filter`,
			wantModifiers: []string{"script"},
		},
		{
			name:        "regexp rule without modifiers",
			rawRule:     `/ads\d+/`,
			wantPattern: `/ads\d+/`,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("parses %s", tt.name), func(t *testing.T) {
			t.Parallel()

			gotPattern, gotModifiers := parseRuleParts(tt.rawRule)
			if gotPattern != tt.wantPattern {
				t.Errorf("pattern = %q, want %q", gotPattern, tt.wantPattern)
			}
			if !reflect.DeepEqual(gotModifiers, tt.wantModifiers) {
				t.Errorf("modifiers = %#v, want %#v", gotModifiers, tt.wantModifiers)
			}
		})
	}
}

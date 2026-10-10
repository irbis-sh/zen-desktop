package filter

import (
	"slices"
	"strings"
	"testing"
)

func TestEvalCondition(t *testing.T) {
	t.Parallel()

	// Only adguard is declared.
	cases := []struct {
		expr    string
		want    bool
		wantErr bool
	}{
		{expr: " adguard", want: true},
		{expr: " !adguard", want: false},
		{expr: " (adguard && adguard_app_ios)", want: false},
		{expr: " ext_abp", want: false},
		{expr: " !ext_abp", want: true},
		{expr: " !!ext_abp", want: false},
		{expr: " (adguard_ext_safari || adguard_app_ios || adguard_ext_android_cb)", want: false},
		{expr: " (!adguard_app_windows && !adguard_app_mac)", want: true},
		{expr: " !a || b && c", want: true},
		{expr: " !a && b || !c", want: true},
		{expr: " !(a || b)", want: true},
		{expr: " a || b", want: false},
		{expr: "(!ext_ublock)", want: true},
		{expr: " ext_2x", want: false},
		{expr: "\t(ext_abp ||\tadguard)", want: true},

		{expr: "", wantErr: true},
		{expr: " (a", wantErr: true},
		{expr: " a)", wantErr: true},
		{expr: " a & b", wantErr: true},
		{expr: " a || ", wantErr: true},
		{expr: " !", wantErr: true},
		{expr: " " + strings.Repeat("!", maxConditionDepth+1) + "a", wantErr: true},
	}
	for _, c := range cases {
		got, err := evalCondition(c.expr)
		if (err != nil) != c.wantErr {
			t.Errorf("evalCondition(%q): err = %v, wantErr %v", c.expr, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("evalCondition(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestDirectivesSkip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		list     string
		want     []string
		wantErr  bool
		wantOpen int
	}{
		{
			name: "TrueBlockWithElse",
			list: "!#if !ext_abp\nkept\n!#else\nskipped\n!#endif\nafter",
			want: []string{"kept", "after"},
		},
		{
			name: "FalseBlockWithElse",
			list: "!#if ext_abp\nskipped\n!#else\nkept\n!#endif\nafter",
			want: []string{"kept", "after"},
		},
		{
			// The inner !#endif must close the inner block, not the outer
			// one, so "skipped 2" stays hidden.
			name: "NestedInFalseBranch",
			list: "!#if ext_abp\n!#if !ext_abp\nskipped 1\n!#endif\nskipped 2\n!#endif\nafter",
			want: []string{"after"},
		},
		{
			name: "ElseUnderInactiveParent",
			list: "!#if ext_abp\n!#if ext_abp\nskipped 1\n!#else\nskipped 2\n!#endif\n!#endif\nafter",
			want: []string{"after"},
		},
		{
			name: "NestedInTrueBranch",
			list: "!#if !ext_abp\n!#if ext_abp\nskipped\n!#else\nkept\n!#endif\n!#endif",
			want: []string{"kept"},
		},
		{
			name: "StrayElseAndEndif",
			list: "!#else\nkept 1\n!#endif\nkept 2",
			want: []string{"kept 1", "kept 2"},
		},
		{
			name:    "MalformedConditionSkipsBlock",
			list:    "!#if (ext_abp\nskipped\n!#endif\nafter",
			want:    []string{"after"},
			wantErr: true,
		},
		{
			name:    "MalformedConditionWithElse",
			list:    "!#if !(ext_abp\nskipped\n!#else\nkept\n!#endif",
			want:    []string{"kept"},
			wantErr: true,
		},
		{
			name:    "MalformedConditionInFalseBlock",
			list:    "!#if ext_abp\n!#if (ext_abp\nskipped\n!#endif\n!#endif\nafter",
			want:    []string{"after"},
			wantErr: true,
		},
		{
			name:     "UnclosedFalseBlock",
			list:     "!#if ext_abp\nskipped 1\nskipped 2",
			want:     nil,
			wantOpen: 1,
		},
		{
			name:     "UnclosedNestedBlocks",
			list:     "!#if adguard\nkept\n!#if ext_abp\nskipped",
			want:     []string{"kept"},
			wantOpen: 2,
		},
		{
			name: "NotDirectives",
			list: "!# if ext_abp\n!#iframe\n!+ PLATFORM(ios)\nkept",
			want: []string{"!# if ext_abp", "!#iframe", "!+ PLATFORM(ios)", "kept"},
		},
		{
			name: "IfWithoutSpace",
			list: "!#if!ext_abp\nkept\n!#endif",
			want: []string{"kept"},
		},
		{
			name: "EndifWithRemark",
			list: "!#if ext_abp\nskipped\n!#endif ! iOS only\nafter",
			want: []string{"after"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			var d directives
			var got []string
			var gotErr bool
			for line := range strings.Lines(c.list) {
				line = strings.TrimSpace(line)
				skip, err := d.skip(line)
				gotErr = gotErr || err != nil
				if !skip {
					got = append(got, line)
				}
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
			if gotErr != c.wantErr {
				t.Errorf("got error %v, want %v", gotErr, c.wantErr)
			}
			if got := d.open(); got != c.wantOpen {
				t.Errorf("got %d open blocks, want %d", got, c.wantOpen)
			}
		})
	}
}

package networkrules

import (
	"reflect"
	"testing"
)

func TestSplitModifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want []string
	}{
		{
			in:   "",
			want: []string{""},
		},
		{
			in:   ",",
			want: []string{"", ""},
		},
		{
			in:   ",,",
			want: []string{"", "", ""},
		},
		{
			in:   "a,b",
			want: []string{"a", "b"},
		},
		{
			in:   "a,,b",
			want: []string{"a", "", "b"},
		},
		{
			in:   "a,",
			want: []string{"a", ""},
		},
		{
			in:   ",a",
			want: []string{"", "a"},
		},
		{
			in:   `a\,b,c`,
			want: []string{"a,b", "c"},
		},
		{
			in:   `a\\,b,c`,
			want: []string{`a\`, "b", "c"},
		},
		{
			in:   `a\,`,
			want: []string{"a,"},
		},
		{
			in:   `\,`,
			want: []string{","},
		},
		{
			in:   `x\y`,
			want: []string{`x\y`},
		},
		{
			in:   `a\`,
			want: []string{`a\`},
		},
		{
			in:   `a\\`,
			want: []string{`a\`},
		},
		{
			// https://github.com/irbis-sh/zen-desktop/issues/509
			in:   `removeparam=/^WT\..*$/i,thirdparty`,
			want: []string{`removeparam=/^WT\..*$/i`, "thirdparty"},
		},
		{
			in:   `jsonprune=$['adPlacements'\,'adSlots'\,'playerAds']`,
			want: []string{`jsonprune=$['adPlacements','adSlots','playerAds']`},
		},
	}

	for _, tc := range tests {
		got := splitModifiers(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitModifiers(%q)=%#v, want=%#v", tc.in, got, tc.want)
		}
	}
}

package scriptlet

import (
	"slices"
	"testing"
)

func TestParseCanonicalArgList(t *testing.T) {
	t.Parallel()

	t.Run("decodes well-formed argument lists", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			input    string
			expected []string
		}{
			{`'set-constant', 'first', 'false'`, []string{"set-constant", "first", "false"}},
			{`'single'`, []string{"single"}},
			{`"set-constant",   "first",	"false"`, []string{"set-constant", "first", "false"}},
			{`''`, []string{""}},
			// Commas inside quotes are literal.
			{`'aopr', 'a,b,c'`, []string{"aopr", "a,b,c"}},
			// A backslash escapes the bounding quote; other quote style needs no escaping.
			{`'rpnt', 'if (a === \'hidden\')'`, []string{"rpnt", "if (a === 'hidden')"}},
			{`"rpnt", "adv_src: '"`, []string{"rpnt", "adv_src: '"}},
			// All other backslashes are literal bytes.
			{`'acis', 'document.createElement', '/html-load\.com|if\(await eval/'`,
				[]string{"acis", "document.createElement", `/html-load\.com|if\(await eval/`}},
			{`'prevent-xhr', '/atr\?.+?&rt=\d+\.\d+/ method:POST'`,
				[]string{"prevent-xhr", `/atr\?.+?&rt=\d+\.\d+/ method:POST`}},
			{`"prevent-setInterval", "/\['\\x[\s\S]*?checkInterval/"`,
				[]string{"prevent-setInterval", `/\['\\x[\s\S]*?checkInterval/`}},
			// An escaped backslash does not escape a following quote.
			{`'ends with backslash\\', 'x'`, []string{`ends with backslash\\`, "x"}},
		}

		for _, test := range testCases {
			got, err := parseCanonicalArgList(test.input)
			if err != nil {
				t.Fatalf("parseCanonicalArgList(%q) returned an error: %v", test.input, err)
			}
			if !slices.Equal(got, test.expected) {
				t.Errorf("parseCanonicalArgList(%q) = %q, want %q", test.input, got, test.expected)
			}
		}
	})

	t.Run("errors on malformed argument lists", func(t *testing.T) {
		t.Parallel()

		testCases := []string{
			``,
			` `,
			`unquoted`,
			`'unclosed`,
			`'a' 'missing comma'`,
			`'a', ,'b'`,
			`'a', b'`,
			`'a', "inconsistent quotes"`,
			`'trailing comma',`,
			"`backticks unsupported`",
		}

		for _, test := range testCases {
			if _, err := parseCanonicalArgList(test); err == nil {
				t.Errorf("parseCanonicalArgList(%q) did not return an error", test)
			}
		}
	})
}

func TestParseUboArgList(t *testing.T) {
	t.Parallel()

	t.Run("decodes well-formed argument lists", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			input    string
			expected []string
		}{
			{`set-constant, first, false`, []string{"set-constant", "first", "false"}},
			{`acis`, []string{"acis"}},
			// Escaped commas are literal; the backslash is removed.
			{`aeld, /^load[A-Za-z]{12\,}/`, []string{"aeld", "/^load[A-Za-z]{12,}/"}},
			{`rpnt, script, /vastURL:.*?'\,/, vastURL: ''\,`,
				[]string{"rpnt", "script", `/vastURL:.*?',/`, "vastURL: '',"}},
			// All other backslashes are literal bytes.
			{`no-xhr-if, /atr\?.+?&rt=\d+\.\d+/ method:POST`,
				[]string{"no-xhr-if", `/atr\?.+?&rt=\d+\.\d+/ method:POST`}},
			// Quoted arguments: quotes are stripped, commas inside are literal.
			{`href-sanitizer, 'a[href*=".com/a?"][href*="&r=http"]', ?r`,
				[]string{"href-sanitizer", `a[href*=".com/a?"][href*="&r=http"]`, "?r"}},
			{`trusted-set, prop, '{"a": "b, c"}'`, []string{"trusted-set", "prop", `{"a": "b, c"}`}},
			{"aopr, `backtick, quoted`", []string{"aopr", "backtick, quoted"}},
			// A backslash escapes the bounding quote inside a quoted argument.
			{`rpnt, script, 'a === \'hidden\''`, []string{"rpnt", "script", "a === 'hidden'"}},
			// A quote not followed by a separator does not start a quoted argument.
			{`aopr, 'abc'def, x`, []string{"aopr", "'abc'def", "x"}},
			// Empty argument between separators.
			{`trusted-rpnt, script, , fallback`, []string{"trusted-rpnt", "script", "", "fallback"}},
			// A trailing separator followed by whitespace yields a final empty
			// argument; without whitespace it does not.
			{`set-local-storage-item, sdfgh45678, `, []string{"set-local-storage-item", "sdfgh45678", ""}},
			{`aost, Math.random,`, []string{"aost", "Math.random"}},
			{`'quoted', `, []string{"quoted", ""}},
		}

		for _, test := range testCases {
			got, err := parseUboArgList(test.input)
			if err != nil {
				t.Fatalf("parseUboArgList(%q) returned an error: %v", test.input, err)
			}
			if !slices.Equal(got, test.expected) {
				t.Errorf("parseUboArgList(%q) = %q, want %q", test.input, got, test.expected)
			}
		}
	})

	t.Run("errors on empty argument lists", func(t *testing.T) {
		t.Parallel()

		for _, test := range []string{``, ` `, "\t"} {
			if _, err := parseUboArgList(test); err == nil {
				t.Errorf("parseUboArgList(%q) did not return an error", test)
			}
		}
	})
}

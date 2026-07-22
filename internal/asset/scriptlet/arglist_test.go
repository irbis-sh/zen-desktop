package scriptlet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewArgList(t *testing.T) {
	t.Parallel()

	t.Run("encodes arguments as comma-joined JSON strings", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			args     []string
			expected string
		}{
			{[]string{"set-constant", "first", "false"}, `"set-constant","first","false"`},
			{[]string{"abort-on-property-read"}, `"abort-on-property-read"`},
			{[]string{"acis", `/html-load\.com|if\(await eval/`}, `"acis","/html-load\\.com|if\\(await eval/"`},
			{[]string{"aopr", ""}, `"aopr",""`},
		}

		for _, test := range testCases {
			got, err := newArgList(test.args)
			if err != nil {
				t.Fatalf("newArgList(%q) returned an error: %v", test.args, err)
			}
			if string(got) != test.expected {
				t.Errorf("newArgList(%q) = %q, want %q", test.args, got, test.expected)
			}
		}
	})

	t.Run("round-trips arguments through JSON parsing", func(t *testing.T) {
		t.Parallel()

		// Checks the framing contract: comma-joined encoded arguments form a
		// valid array whose elements decode back to the original bytes. That
		// the browser's JS parser agrees with the JSON decoder used here is
		// assumed, not tested: JSON is a syntactic subset of ECMAScript since
		// ES2019, and encoding/json escapes the historical exceptions
		// (U+2028/U+2029).
		testCases := [][]string{
			{"acis", "document.createElement", `/html-load\.com|error-report\.com|if\(await eval/`},
			{"no-xhr-if", `/atr\?.+?&rt=\d+\.\d+.+?&muted=\d(&vis=3)?&docid=/ method:POST`},
			{"rpnt", "script", `back\slash at the end\`},
			{"set-constant", "console.log", "trueFunc", `quotes ' " and <angle> brackets`},
		}

		for _, args := range testCases {
			al, err := newArgList(args)
			if err != nil {
				t.Fatalf("newArgList(%q) returned an error: %v", args, err)
			}
			var decoded []string
			if err := json.Unmarshal([]byte("["+string(al)+"]"), &decoded); err != nil {
				t.Fatalf("newArgList(%q) produced invalid JSON: %v", args, err)
			}
			if len(decoded) != len(args) {
				t.Fatalf("round-trip of %q produced %d args, want %d", args, len(decoded), len(args))
			}
			for i := range args {
				if decoded[i] != args[i] {
					t.Errorf("round-trip of %q: arg %d = %q, want %q", args, i, decoded[i], args[i])
				}
			}
		}
	})

	t.Run("escapes characters that could break out of the script context", func(t *testing.T) {
		t.Parallel()

		al, err := newArgList([]string{"aopr", "</script><script>alert(1)</script>"})
		if err != nil {
			t.Fatalf("newArgList returned an error: %v", err)
		}
		if strings.Contains(string(al), "<") || strings.Contains(string(al), ">") {
			t.Errorf("newArgList left angle brackets unescaped: %q", al)
		}
	})
}

func TestGenerateInjection(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	al, err := newArgList([]string{"abort-on-property-read", `/\d+\.\d+/`})
	if err != nil {
		t.Fatalf("newArgList returned an error: %v", err)
	}
	if err := al.GenerateInjection(&b); err != nil {
		t.Fatalf("GenerateInjection returned an error: %v", err)
	}

	expected := `try{scriptlet("abort-on-property-read","/\\d+\\.\\d+/")}catch(ex){console.error(ex);}`
	if b.String() != expected {
		t.Errorf("GenerateInjection wrote %q, want %q", b.String(), expected)
	}
}

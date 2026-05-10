package scriptlet

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()

	t.Run("proper normalization of arg lists in expected format", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			input    string
			expected string
		}{
			{`"arg1", "arg2"`, `"arg1","arg2"`},
			{`"arg1",          "arg2"`, `"arg1","arg2"`},
			{`"arg"`, `"arg"`},
			{`'arg1', 'arg2'`, `'arg1','arg2'`},
		}

		for _, test := range testCases {
			argList := argList(test.input)
			got, err := argList.Normalize()
			if err != nil {
				t.Fatalf("normalize(%q) returned an error: %v", test.input, err)
			}
			if string(got) != test.expected {
				t.Errorf("normalize(%q) = %q, want %q", test.input, got, test.expected)
			}
		}
	})

	t.Run("error on improper formatting", func(t *testing.T) {
		t.Parallel()

		testCases := []string{
			`"arg1", "arg2`,
			`"`,
			`"""`,
			` `,
			`"arg1\", "arg2"`,
			`"arg1", ,"arg2"`,
		}

		for _, test := range testCases {
			argList := argList(test)
			_, err := argList.Normalize()
			if err == nil {
				t.Errorf("normalize(%q) did not return an error", test)
			}
		}
	})
}

func TestValidateJSString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input string
		valid bool
	}{
		{`"valid"`, true},
		{`'valid'`, true},
		{`"he said \"hello\""`, true},
		{`'it\'s fine'`, true},
		{`"\"nested\""`, true},
		{`"backslash at end\\"`, true},
		{`"\\\\\""`, true},
		{`"single'quote"`, true},
		{`"valid\\\\"`, true},
		{`"new\nline"`, true},
		{`"'double' inside"`, true},
		{`'escaped quote: \\\''`, true},
		{`"emoji \u2764"`, true},
		{`"double backslash \\\\"`, true},
		{`"tab\tcharacter"`, true},
		{`"empty \0"`, true},
		{`"weird\\'mix"`, true},
		{`"quotes inside \\\""`, true},
		{`'quote mismatch"`, false},
		{`"escaped end quote: \\""`, false},
		{`no quotes`, false},
		{`"double double quote""`, false},
		{`'double single quote''`, false},
		{`"no end quote`, false},
		{`no start quote"`, false},
		{`"`, false},
		{`"\\""`, false},
	}

	for _, test := range testCases {
		if got := isValidJSString(test.input); got != test.valid {
			t.Errorf("validateJSString(%q) = %t, want %t", test.input, got, test.valid)
		}
	}
}

func TestIsTrusted(t *testing.T) {
	t.Parallel()

	t.Run("returns false for non-'trusted-' name", func(t *testing.T) {
		t.Parallel()

		argList, err := argList(`"test", "1", "2"`).Normalize()
		if err != nil {
			t.Fatalf("unexpected normalization error: %v", err)
		}

		if argList.IsTrusted() {
			t.Error("IsTrusted() = true, want false")
		}
	})

	t.Run("returns false and does not panic on empty name", func(t *testing.T) {
		t.Parallel()

		argList, err := argList(`""`).Normalize()
		if err != nil {
			t.Fatalf("unexpected normalization error: %v", err)
		}

		if argList.IsTrusted() {
			t.Error("IsTrusted() = true, want false")
		}
	})

	t.Run("returns true on 'trusted-' name", func(t *testing.T) {
		t.Parallel()

		argList, err := argList(`"trusted-test"`).Normalize()
		if err != nil {
			t.Fatalf("unexpected normalization error: %v", err)
		}

		if !argList.IsTrusted() {
			t.Error("IsTrusted() = false, want true")
		}
	})
}

package rulemodifiers

import "testing"

func TestParseRegexp(t *testing.T) {
	t.Parallel()

	t.Run("returns nil if input is empty", func(t *testing.T) {
		t.Parallel()

		regexp, err := parseRegexp("")
		if err != nil {
			t.Errorf("expected error to be nil, got: %v", err)
		}
		if regexp != nil {
			t.Errorf("expected regexp to be nil, got: %v", regexp)
		}
	})

	t.Run("returns nil if input is not a regular expression", func(t *testing.T) {
		t.Parallel()

		regexp, err := parseRegexp("notaregexp")
		if err != nil {
			t.Errorf("expected error to be nil, got: %v", err)
		}
		if regexp != nil {
			t.Errorf("expected regexp to be nil, got: %v", regexp)
		}
	})

	t.Run("returns regexp if input is valid", func(t *testing.T) {
		t.Parallel()

		regexp, err := parseRegexp("/test/")
		if err != nil {
			t.Errorf("expected error to be nil, got: %v", err)
		}
		if regexp == nil {
			t.Error("expected regexp to be non-nil")
		}
	})

	t.Run("correctly interprets JS-style case insensitivity flag", func(t *testing.T) {
		t.Parallel()

		regexp, err := parseRegexp("/test/i")
		if err != nil {
			t.Errorf("expected error to be nil, got: %v", err)
		}
		if regexp == nil {
			t.Errorf("expected regexp to be non-nil, got nil")
		}

		if !regexp.MatchString("test") {
			t.Error("expected regexp to match \"test\"")
		}
		if !regexp.MatchString("TEST") {
			t.Error("expected regexp to match \"TEST\"")
		}
		if regexp.MatchString("prod") {
			t.Error("expected regexp not to match \"prod\"")
		}
	})
}

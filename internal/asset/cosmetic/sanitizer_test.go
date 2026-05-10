package cosmetic

import (
	"testing"
)

func TestSanitizer(t *testing.T) {
	t.Parallel()

	t.Run("simple selector is not sanitized", func(t *testing.T) {
		t.Parallel()

		selector := "body"
		sanitized, err := sanitizeCSSSelector(selector)
		if err != nil {
			t.Fatal(err)
		}

		if sanitized != "body" {
			t.Errorf("expected %q, got %q", selector, sanitized)
		}
	})

	t.Run("Valid Complex Selectors", func(t *testing.T) {
		selector := `body > div[id^="ai-adb-"][style^="position: fixed; top:"][style*="z-index: 9999"]:hover`
		if _, err := sanitizeCSSSelector(selector); err != nil {
			t.Errorf("Expected valid selector, got error: %v", err)
		}

		selector = `a[href^="https://"][data-info="some:info"][class~="button active"]`
		if _, err := sanitizeCSSSelector(selector); err != nil {
			t.Errorf("Expected valid selector, got error: %v", err)
		}

		selector = `ul.menu > li[class*="dropdown"] ul li a[href*="contact"]`
		if _, err := sanitizeCSSSelector(selector); err != nil {
			t.Errorf("Expected valid selector, got error: %v", err)
		}
	})

	t.Run("Dangerous Characters Outside Quotes", func(t *testing.T) {
		selector := `div } body { color: red; }`
		if _, err := sanitizeCSSSelector(selector); err == nil {
			t.Error("Expected error for selector with '}' outside quotes, got none")
		}

		selector = `span; @import 'evil.css';`
		if _, err := sanitizeCSSSelector(selector); err == nil {
			t.Error("Expected error for selector with ';' and '@import' outside quotes, got none")
		}

		selector = `div /* comment */ span`
		if _, err := sanitizeCSSSelector(selector); err == nil {
			t.Error("Expected error for '/*' outside quotes, got none")
		}
	})

	t.Run("Balanced vs Unbalanced Quotes/Brackets", func(t *testing.T) {
		selector := `div[class^="header"][data-role='main']`
		if _, err := sanitizeCSSSelector(selector); err != nil {
			t.Errorf("Expected valid selector, got error: %v", err)
		}

		selector = `a[href^="https://]`
		if _, err := sanitizeCSSSelector(selector); err == nil {
			t.Error("Expected error for unbalanced quotes, got none")
		}

		selector = `div[class^="header"`
		if _, err := sanitizeCSSSelector(selector); err == nil {
			t.Error("Expected error for unbalanced brackets, got none")
		}
	})

	t.Run("Unicode Escapes", func(t *testing.T) {
		selector := `div[class^="\0061"]`
		sanitized, err := sanitizeCSSSelector(selector)
		if err != nil {
			t.Errorf("Expected valid selector, got error: %v", err)
		} else if sanitized != `div[class^="a"]` {
			t.Errorf("Expected decoded Unicode escape, got %v", sanitized)
		}

		selector = `span[data-test="\0062\0063"]`
		sanitized, err = sanitizeCSSSelector(selector)
		if err != nil {
			t.Errorf("Expected valid selector, got error: %v", err)
		} else if sanitized != `span[data-test="bc"]` {
			t.Errorf("Expected 'bc' after decoding, got %v", sanitized)
		}

		selector = `p.note`
		if _, err := sanitizeCSSSelector(selector); err != nil {
			t.Errorf("Expected valid selector without Unicode escapes, got error: %v", err)
		}
	})
}

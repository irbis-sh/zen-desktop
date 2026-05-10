package hostmatch_test

import (
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/hostmatch"
)

func TestHostMatcherPublic(t *testing.T) {
	t.Parallel()

	t.Run("makes a single match", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("example.com", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		res := hm.Get("example.com")
		if len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result to be 'test', got %v", res)
		}
	})

	t.Run("makes a match based on a pattern containing a wildcard", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("example.*", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if res := hm.Get("example.com"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for example.com to be 'test', got %v", res)
		}

		if res := hm.Get("example.co.uk"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for example.co.uk to be 'test', got %v", res)
		}
	})

	t.Run("makes a subdomain match", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("example.com", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if res := hm.Get("sub.example.com"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for sub.example.com to be 'test', got %v", res)
		}

		if res := hm.Get("sub3.sub2.sub1.example.com"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for 'sub3.sub2.sub1.example.com' to be 'test', got %v", res)
		}
	})

	t.Run("matches rule for multiple domains", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("example.com,mysal.kz,example.net", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if res := hm.Get("example.com"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for example.com to be 'test', got %v", res)
		}

		if res := hm.Get("mysal.kz"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for mysal.kz to be 'test', got %v", res)
		}

		if res := hm.Get("example.net"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for example.net to be 'test', got %v", res)
		}
	})

	t.Run("exception rule neutralizes primary rule", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("example.com", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}
		if err := hm.AddExceptionRule("example.com", "test"); err != nil {
			t.Fatalf("failed to add exception rule: %v", err)
		}

		if res := hm.Get("example.com"); len(res) != 0 {
			t.Errorf("expected result to be empty, got %v", res)
		}
	})

	t.Run("tilde exception neutralizes primary rule", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("example.com,~sub.example.com", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if res := hm.Get("sub.example.com"); len(res) != 0 {
			t.Errorf("expected result for sub.example.com to be empty, got %v", res)
		}
		if res := hm.Get("notsub.example.com"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for notsub.example.com to be 'test', got %v", res)
		}
	})

	t.Run("rule with empty hostnamePatterns matches any hostname", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if res := hm.Get("example.com"); len(res) == 0 || res[0] != "test" {
			t.Errorf("expected result for example.com to be 'test', got %v", res)
		}
	})

	t.Run("generic exception rule neutralizes generic primary rule", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}
		if err := hm.AddExceptionRule("", "test"); err != nil {
			t.Fatalf("failed to add exception rule: %v", err)
		}

		if res := hm.Get("example.com"); len(res) != 0 {
			t.Errorf("expected result to be empty, got %v", res)
		}
	})

	t.Run("tilde exception neutralizes deep subdomains", func(t *testing.T) {
		t.Parallel()

		hm := hostmatch.NewHostMatcher[string]()
		if err := hm.AddPrimaryRule("example.com,~sub.example.com", "test"); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if res := hm.Get("deep.sub.example.com"); len(res) != 0 {
			t.Errorf("expected result for deep.sub.example.com to be empty, got %v", res)
		}
	})
}

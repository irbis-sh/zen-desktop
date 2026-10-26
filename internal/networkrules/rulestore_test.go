package networkrules

import (
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestRuleStoreMatch(t *testing.T) {
	t.Parallel()

	s := newRuleStore[string]()
	for _, r := range []struct{ pattern, value string }{
		{"", "generic"},
		{"", "generic-rejected"},
		{"||example.com^", "tree"},
		{"||example.com^", "tree-rejected"},
		{"||other.com^", "tree-unmatched"},
		{`/banner\d+/`, "regexp"},
		{`/banner\d+/`, "regexp-rejected"},
		{`/popup\d+/`, "regexp-unmatched"},
	} {
		if err := s.Insert(r.pattern, r.value); err != nil {
			t.Fatalf("insert %q: %v", r.pattern, err)
		}
	}

	got := s.Match("https://example.com/banner1", func(v string) bool {
		return !strings.HasSuffix(v, "-rejected")
	})
	slices.Sort(got)
	if want := []string{"generic", "regexp", "tree"}; !slices.Equal(got, want) {
		t.Errorf("Match = %q, want %q", got, want)
	}
}

// TestRuleStoreConcurrentInsertAndMatch is meant for -race: the block page's
// allow button inserts rules of every pattern kind while requests are matched.
func TestRuleStoreConcurrentInsertAndMatch(t *testing.T) {
	t.Parallel()

	s := newRuleStore[string]()
	s.Compact()

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 200 {
			for _, pattern := range []string{"", `/banner\d+/`, "||ads.example.com^", "/ads/*"} {
				if err := s.Insert(pattern, pattern); err != nil {
					t.Errorf("insert %q: %v", pattern, err)
					return
				}
			}
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 200 {
				s.Match("https://ads.example.com/ads/banner1", func(string) bool { return true })
			}
		})
	}
	wg.Wait()
}

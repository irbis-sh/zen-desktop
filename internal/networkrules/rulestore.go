package networkrules

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/irbis-sh/zen-desktop/internal/ruletree"
	"github.com/irbis-sh/zen-desktop/internal/trimslice"
)

// ruleStore matches URLs against ad-block style patterns with associated data.
type ruleStore[T comparable] struct {
	// mu guards tree, generic and regexp. Rules are inserted from several
	// goroutines while lists load, and while requests are matched, e.g. from
	// the block page's allow button.
	mu   sync.RWMutex
	tree *ruletree.Tree[T]
	// generic is rules with an empty pattern, which match for every URL.
	generic []T
	regexp  []regexpRule[T]
}

type regexpRule[T comparable] struct {
	regexp *regexp.Regexp
	value  T
}

func newRuleStore[T comparable]() *ruleStore[T] {
	return &ruleStore[T]{
		tree: ruletree.New[T](),
	}
}

func (s *ruleStore[T]) Insert(pattern string, v T) error {
	// The pattern is either generic (empty), regexp, or regular.
	switch {
	case pattern == "":
		s.mu.Lock()
		s.generic = append(s.generic, v)
		s.mu.Unlock()
	case len(pattern) > 1 && pattern[0] == '/' && pattern[len(pattern)-1] == '/':
		body := pattern[1 : len(pattern)-1]
		if body == "" {
			return fmt.Errorf("empty regexp rule")
		}
		re, err := regexp.Compile(body)
		if err != nil {
			return fmt.Errorf("compile regexp rule: %w", err)
		}
		s.mu.Lock()
		s.regexp = append(s.regexp, regexpRule[T]{
			regexp: re,
			value:  v,
		})
		s.mu.Unlock()
	default:
		s.mu.Lock()
		s.tree.Insert(pattern, v)
		s.mu.Unlock()
	}

	return nil
}

// Match returns the values whose pattern matches url and for which keep
// returns true. keep may be called for values whose pattern does not match:
// it runs before the regexp match, which costs more. keep runs under the
// store's read lock, so it must not call Insert or Compact.
func (s *ruleStore[T]) Match(url string, keep func(T) bool) []T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var res []T
	for _, v := range s.generic {
		if keep(v) {
			res = append(res, v)
		}
	}
	for _, v := range s.tree.Get(url) {
		if keep(v) {
			res = append(res, v)
		}
	}
	for _, rule := range s.regexp {
		if keep(rule.value) && rule.regexp.MatchString(url) {
			res = append(res, rule.value)
		}
	}
	return res
}

func (s *ruleStore[T]) Compact() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.generic = trimslice.TrimSlice(s.generic)
	s.regexp = trimslice.TrimSlice(s.regexp)
	s.tree.Compact()
}

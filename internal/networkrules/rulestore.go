package networkrules

import (
	"fmt"
	"regexp"

	"github.com/irbis-sh/zen-desktop/internal/ruletree"
	"github.com/irbis-sh/zen-desktop/internal/trimslice"
)

// ruleStore matches URLs against ad-block style patterns with associated data.
type ruleStore[T comparable] struct {
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
		s.generic = append(s.generic, v)
	case len(pattern) > 1 && pattern[0] == '/' && pattern[len(pattern)-1] == '/':
		body := pattern[1 : len(pattern)-1]
		if body == "" {
			return fmt.Errorf("empty regexp rule")
		}
		re, err := regexp.Compile(body)
		if err != nil {
			return fmt.Errorf("compile regexp rule: %w", err)
		}
		s.regexp = append(s.regexp, regexpRule[T]{
			regexp: re,
			value:  v,
		})
	default:
		s.tree.Insert(pattern, v)
	}

	return nil
}

func (s *ruleStore[T]) Get(url string) []T {
	matches := s.tree.Get(url)
	res := make([]T, 0, len(s.generic)+len(matches))
	res = append(res, s.generic...)
	res = append(res, matches...)
	for _, rule := range s.regexp {
		if rule.regexp.MatchString(url) {
			res = append(res, rule.value)
		}
	}
	return res
}

func (s *ruleStore[T]) Compact() {
	s.generic = trimslice.TrimSlice(s.generic)
	s.regexp = trimslice.TrimSlice(s.regexp)
	s.tree.Compact()
}

package networkrules

import (
	"github.com/irbis-sh/zen-desktop/internal/ruletree"
	"github.com/irbis-sh/zen-desktop/internal/trimslice"
)

type treeRuleStore[T comparable] struct {
	tree    *ruletree.Tree[T]
	generic []T
}

func newRuleStore[T comparable]() *treeRuleStore[T] {
	return &treeRuleStore[T]{
		tree: ruletree.New[T](),
	}
}

func (s *treeRuleStore[T]) Insert(pattern string, v T) {
	if pattern == "" {
		s.generic = append(s.generic, v)
		return
	}

	s.tree.Insert(pattern, v)
}

func (s *treeRuleStore[T]) Get(url string) []T {
	matches := s.tree.Get(url)
	res := make([]T, 0, len(s.generic)+len(matches))
	res = append(res, s.generic...)
	res = append(res, matches...)
	return res
}

func (s *treeRuleStore[T]) Compact() {
	s.generic = trimslice.TrimSlice(s.generic)
	s.tree.Compact()
}

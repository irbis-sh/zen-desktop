package ruletree

import "github.com/irbis-sh/zen-desktop/internal/trimslice"

// Compact shrinks internal slice capacities to reduce memory usage.
func (t *Tree[T]) Compact() {
	t.insertMu.Lock()
	defer t.insertMu.Unlock()

	stack := []*node[T]{
		t.anchorRoot,
		t.domainBoundaryRoot,
		t.root,
	}

	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if n.isLeaf() {
			n.leaf = trimslice.TrimSlice(n.leaf)
		}

		n.edges = trimslice.TrimSlice(n.edges)
		n.prefix = trimslice.TrimSlice(n.prefix)

		if n.wildcard != nil {
			stack = append(stack, n.wildcard)
		}
		if n.separator != nil {
			stack = append(stack, n.separator)
		}
		if n.anchor != nil {
			stack = append(stack, n.anchor)
		}
		for _, e := range n.edges {
			stack = append(stack, e.node)
		}
	}
}

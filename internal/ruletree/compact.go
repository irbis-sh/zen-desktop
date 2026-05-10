package ruletree

// Compact shrinks internal slice capacities to reduce memory usage.
func (t *Tree[T]) Compact() {
	t.insertMu.Lock()
	defer t.insertMu.Unlock()

	t.generic = trimSlice(t.generic)

	stack := []*node[T]{
		t.anchorRoot,
		t.domainBoundaryRoot,
		t.root,
	}

	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if n.isLeaf() {
			n.leaf = trimSlice(n.leaf)
		}

		n.edges = trimSlice(n.edges)
		n.prefix = trimSlice(n.prefix)

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

func trimSlice[T any](s []T) []T {
	if len(s) == cap(s) {
		return s
	}

	newSlice := make([]T, len(s))
	copy(newSlice, s)
	return newSlice
}

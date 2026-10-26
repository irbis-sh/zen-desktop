package ruletree

import (
	"sort"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/ruletree/byteset"
)

type edge[T Data] struct {
	label token
	node  *node[T]
}

type node[T Data] struct {
	// leaf stores a possible leaf.
	leaf []T

	// prefix is the common prefix: a substring of a tokenized pattern, so a
	// pattern's nodes share its memory. Copying it, e.g. to trim it in
	// Compact, would undo that saving.
	prefix string

	// edges stores child edges sorted by label.
	edges []edge[T]
}

func (n *node[T]) isLeaf() bool {
	return n.leaf != nil
}

func (n *node[T]) addEdge(label token, e *node[T]) {
	idx := sort.Search(len(n.edges), func(i int) bool {
		return n.edges[i].label >= label
	})

	n.edges = append(n.edges, edge[T]{})
	copy(n.edges[idx+1:], n.edges[idx:])
	n.edges[idx] = edge[T]{label, e}
}

func (n *node[T]) updateEdge(label token, node *node[T]) {
	idx := sort.Search(len(n.edges), func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < len(n.edges) && n.edges[idx].label == label {
		n.edges[idx].node = node
	}
}

// getEdge scans linearly: most nodes have at most a few edges, where a
// binary search measured about 15% slower on BenchmarkMatch.
func (n *node[T]) getEdge(label token) *node[T] {
	for _, e := range n.edges {
		if e.label == label {
			return e.node
		}
		if e.label > label {
			break
		}
	}
	return nil
}

// traverser holds the state for a single traverse() call.
type traverser[T Data] struct {
	data []T
	n    *node[T]
}

func (n *node[T]) traverse(url string) []T {
	t := traverser[T]{
		n: n,
	}
	t.traversePrefix(n.prefix, url)

	return t.data
}

func (t *traverser[T]) traversePrefix(prefix string, url string) {
	if len(prefix) == 0 {
		if t.n.isLeaf() {
			t.data = append(t.data, t.n.leaf...)
		}
		if url == "" {
			if anchor := t.n.getEdge(tokenAnchor); anchor != nil {
				t.data = append(t.data, anchor.traverse("")...)
			}
			if wildcard := t.n.getEdge(tokenWildcard); wildcard != nil {
				t.data = append(t.data, wildcard.traverse("")...)
			}
			if separator := t.n.getEdge(tokenSeparator); separator != nil {
				t.data = append(t.data, separator.traverse("")...)
			}
		} else {
			firstCh := url[0]
			if isSeparator(firstCh) {
				if separator := t.n.getEdge(tokenSeparator); separator != nil {
					t.data = append(t.data, separator.traverse(url)...)
				}
			}
			if wildcard := t.n.getEdge(tokenWildcard); wildcard != nil {
				t.data = append(t.data, wildcard.traverse(url)...)
			}
			// firstCh may select a special token's edge; see the tokens'
			// comment for why that's harmless.
			if ch := t.n.getEdge(firstCh); ch != nil {
				t.data = append(t.data, ch.traverse(url)...)
			}
		}
		return
	}
	if len(url) == 0 {
		if t.n.isLeaf() && len(prefix) == 1 && (prefix[0] == tokenAnchor || prefix[0] == tokenSeparator || prefix[0] == tokenWildcard) {
			t.data = append(t.data, t.n.leaf...)
		}
		return
	}

	switch prefix[0] {
	case tokenAnchor:
		// An anchor here can only match the end of the URL, handled above.
		return
	case tokenWildcard:
		if len(prefix) == 1 {
			t.traverseWildcardTail(url)
		} else {
			switch prefix[1] {
			case tokenAnchor:
				t.traversePrefix(prefix[1:], "")
			case tokenSeparator:
				for i := 0; i < len(url); i++ {
					if isSeparator(url[i]) {
						t.traversePrefix(prefix[1:], url[i:])
					}
				}
			default:
				target := prefix[1]
				off := 0
				for off < len(url) {
					idx := strings.IndexByte(url[off:], target)
					if idx < 0 {
						break
					}
					t.traversePrefix(prefix[1:], url[off+idx:])
					off += idx + 1
				}
			}
		}
	case tokenSeparator:
		if !isSeparator(url[0]) {
			return
		}
		// Scan forward past all separator chars and make
		// a single recursive call for the boundary, plus one
		// for the remaining prefix at the first non-separator position.
		i := 1
		for i < len(url) && isSeparator(url[i]) {
			i++
		}
		t.traversePrefix(prefix[1:], url[1:])
		if i > 1 {
			t.traversePrefix(prefix[1:], url[i:])
		}
	default:
		if prefix[0] == url[0] {
			t.traversePrefix(prefix[1:], url[1:])
		}
	}
}

// traverseWildcardTail handles a wildcard at the end of a node's prefix.
// Instead of calling traversePrefix("", url[i:]) for every i, it
// collects the set of characters that can actually start a child edge
// and only dispatches on positions where those characters appear.
func (t *traverser[T]) traverseWildcardTail(url string) {
	n := t.n

	// Wildcard matches the entire remaining URL.
	t.traversePrefix("", "")

	separator := n.getEdge(tokenSeparator)
	wildcard := n.getEdge(tokenWildcard)

	// Build a set of the first characters of the node's edges.
	var edgeChars byteset.Set
	for _, e := range n.edges {
		edgeChars.Add(e.label)
	}

	for i := 0; i < len(url); i++ {
		ch := url[i]

		if separator != nil && isSeparator(ch) {
			t.data = append(t.data, separator.traverse(url[i:])...)
		}
		if wildcard != nil {
			t.data = append(t.data, wildcard.traverse(url[i:])...)
		}
		// ch may select a special token's edge; see the tokens' comment for
		// why that's harmless.
		if edgeChars.Has(ch) {
			if child := n.getEdge(ch); child != nil {
				t.data = append(t.data, child.traverse(url[i:])...)
			}
		}
	}
}

var separators byteset.Set

func init() {
	const sepChars = "~:/?#[]@!$&'()*+,;="
	for i := range sepChars {
		ch := sepChars[i]
		separators.Add(ch)
	}
}

func isSeparator(char byte) bool {
	return separators.Has(char)
}

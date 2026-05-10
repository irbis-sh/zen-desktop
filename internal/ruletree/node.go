package ruletree

import (
	"sort"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/ruletree/byteset"
)

type litEdge[T Data] struct {
	label byte
	node  *node[T]
}

type node[T Data] struct {
	// leaf stores a possible leaf.
	leaf []T

	// prefix is the common prefix.
	prefix []token

	// Special-token edges.

	wildcard  *node[T]
	separator *node[T]
	anchor    *node[T]

	// edges stores literal-character edges in sorted order.
	edges []litEdge[T]
}

func (n *node[T]) isLeaf() bool {
	return n.leaf != nil
}

func (n *node[T]) addEdge(label token, e *node[T]) {
	switch label {
	case tokenWildcard:
		n.wildcard = e
		return
	case tokenSeparator:
		n.separator = e
		return
	case tokenAnchor:
		n.anchor = e
		return
	default:
		bLabel := byte(label) // #nosec G115 -- label is guaranteed to be <=255 past this point
		idx := sort.Search(len(n.edges), func(i int) bool {
			return n.edges[i].label >= bLabel
		})

		n.edges = append(n.edges, litEdge[T]{})
		copy(n.edges[idx+1:], n.edges[idx:])
		n.edges[idx] = litEdge[T]{bLabel, e}
	}
}

func (n *node[T]) updateEdge(label token, node *node[T]) {
	switch label {
	case tokenWildcard:
		n.wildcard = node
		return
	case tokenSeparator:
		n.separator = node
		return
	case tokenAnchor:
		n.anchor = node
		return
	default:
		bLabel := byte(label) // #nosec G115 -- label is guaranteed to be <=255 past this point
		idx := sort.Search(len(n.edges), func(i int) bool {
			return n.edges[i].label >= bLabel
		})
		if idx < len(n.edges) && n.edges[idx].label == bLabel {
			n.edges[idx].node = node
		}
	}
}

func (n *node[T]) getEdge(label token) *node[T] {
	switch label {
	case tokenWildcard:
		return n.wildcard
	case tokenSeparator:
		return n.separator
	case tokenAnchor:
		return n.anchor
	default:
		bLabel := byte(label) // #nosec G115 -- label is guaranteed to be <=255 past this point
		idx := sort.Search(len(n.edges), func(i int) bool {
			return n.edges[i].label >= bLabel
		})
		if idx < len(n.edges) && n.edges[idx].label == bLabel {
			return n.edges[idx].node
		}
		return nil
	}
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

func (t *traverser[T]) traversePrefix(prefix []token, url string) {
	if len(prefix) == 0 {
		if t.n.isLeaf() {
			t.data = append(t.data, t.n.leaf...)
		}
		if url == "" {
			if t.n.anchor != nil {
				t.data = append(t.data, t.n.anchor.traverse("")...)
			}
			if t.n.wildcard != nil {
				t.data = append(t.data, t.n.wildcard.traverse("")...)
			}
			if t.n.separator != nil {
				t.data = append(t.data, t.n.separator.traverse("")...)
			}
		} else {
			firstCh := url[0]
			if isSeparator(firstCh) && t.n.separator != nil {
				t.data = append(t.data, t.n.separator.traverse(url)...)
			}
			if t.n.wildcard != nil {
				t.data = append(t.data, t.n.wildcard.traverse(url)...)
			}
			if ch := t.n.getEdge(token(firstCh)); ch != nil {
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
				target := byte(prefix[1]) // #nosec G115 -- literal character tokens are always in ASCII byte range
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
		if prefix[0] == token(url[0]) {
			t.traversePrefix(prefix[1:], url[1:])
		}
	}
}

// traverseWildcardTail handles a wildcard at the end of a node's prefix.
// Instead of calling traversePrefix(nil, url[i:]) for every i, it
// collects the set of characters that can actually start a child edge
// and only dispatches on positions where those characters appear.
func (t *traverser[T]) traverseWildcardTail(url string) {
	n := t.n

	// Wildcard matches the entire remaining URL.
	t.traversePrefix(nil, "")

	hasSep := n.separator != nil
	hasWild := n.wildcard != nil

	// Build a set of literal first-characters from the node's edges.
	var literalSet byteset.Set
	for _, e := range n.edges {
		literalSet.Add(e.label)
	}

	for i := 0; i < len(url); i++ {
		ch := url[i]

		if hasSep && isSeparator(ch) {
			t.data = append(t.data, n.separator.traverse(url[i:])...)
		}
		if hasWild {
			t.data = append(t.data, n.wildcard.traverse(url[i:])...)
		}
		if literalSet.Has(ch) {
			if child := n.getEdge(token(ch)); child != nil {
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

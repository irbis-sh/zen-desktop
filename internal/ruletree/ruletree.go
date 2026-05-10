package ruletree

import (
	"strings"
	"sync"

	"github.com/irbis-sh/zen-desktop/internal/ruletree/byteset"
)

type Data comparable

// Tree is a prefix tree for storing and retrieving data
// associated with adblock-style patterns.
//
// Insert and Compact must not run concurrently with Get.
type Tree[T Data] struct {
	// insertMu protects the tree during inserts.
	insertMu sync.Mutex

	root *node[T]
	// domainBoundaryRoot stores patterns beginning with tokenDomainBoundary (||).
	domainBoundaryRoot *node[T]
	// anchorRoot stores pattern beginning with tokenAnchor (|).
	anchorRoot *node[T]
	// generic is data without an associated pattern. It is returned for every Get call.
	generic []T
}

func New[T Data]() *Tree[T] {
	return &Tree[T]{
		root:               &node[T]{},
		domainBoundaryRoot: &node[T]{},
		anchorRoot:         &node[T]{},
		generic:            make([]T, 0),
	}
}

// Insert adds a pattern with associated data to the tree.
func (t *Tree[T]) Insert(pattern string, v T) {
	if pattern == "" {
		t.insertMu.Lock()
		t.generic = append(t.generic, v)
		t.insertMu.Unlock()
		return
	}

	var parent *node[T]
	var n *node[T]

	tokens := tokenize(pattern)

	for i := 1; i < len(tokens); i++ {
		if tokens[i] == tokenDomainBoundary {
			return
		}
	}

	t.insertMu.Lock()
	defer t.insertMu.Unlock()

	switch tokens[0] {
	case tokenDomainBoundary:
		n, tokens = t.domainBoundaryRoot, tokens[1:]
	case tokenAnchor:
		n, tokens = t.anchorRoot, tokens[1:]
	default:
		n = t.root
	}

	for {
		if len(tokens) == 0 {
			if n.isLeaf() {
				n.leaf = append(n.leaf, v)
			} else {
				n.leaf = []T{v}
			}
			return
		}

		parent = n
		n = n.getEdge(tokens[0])

		if n == nil {
			n := &node[T]{
				prefix: tokens,
				leaf:   []T{v},
			}
			parent.addEdge(tokens[0], n)
			return
		}

		commonPrefix := longestPrefix(tokens, n.prefix)
		if commonPrefix == len(n.prefix) {
			tokens = tokens[commonPrefix:]
			continue
		}

		child := &node[T]{
			prefix: tokens[:commonPrefix],
		}
		parent.updateEdge(tokens[0], child)

		child.addEdge(n.prefix[commonPrefix], n)
		n.prefix = n.prefix[commonPrefix:]

		l := []T{v}
		if commonPrefix == len(tokens) {
			child.leaf = l
		} else {
			n := &node[T]{
				leaf:   l,
				prefix: tokens[commonPrefix:],
			}
			child.addEdge(tokens[commonPrefix], n)
		}
		return
	}
}

// Get retrieves data matching the given URL.
//
// The URL is expected to be a valid URL with scheme and host.
func (t *Tree[T]) Get(url string) []T {
	data := make(map[T]struct{})

	addUnique := func(items []T) {
		for _, item := range items {
			if _, exists := data[item]; !exists {
				data[item] = struct{}{}
			}
		}
	}

	addUnique(t.anchorRoot.traverse(url))
	addUnique(t.root.traverse(url))

	var (
		traverseNext = false

		schemeEnd = strings.Index(url, "://")
		hostStart = schemeEnd + 3
		hostEnd   = strings.IndexAny(url[hostStart:], "/?")
	)
	for i := 1; i < len(url); i++ {
		c := url[i]

		if traverseNext {
			addUnique(t.root.traverse(url[i:]))
			traverseNext = isTraversalMarker(c)
		} else if isTraversalMarker(c) {
			addUnique(t.root.traverse(url[i:]))
			traverseNext = true
		}

		if i == hostStart {
			addUnique(t.domainBoundaryRoot.traverse(url[i:]))
		}
		if i > hostStart && (hostEnd == -1 || i < hostStart+hostEnd) {
			if c == '.' {
				addUnique(t.domainBoundaryRoot.traverse(url[i+1:]))
			}
		}
	}

	result := make([]T, len(t.generic)+len(data))
	copy(result, t.generic)
	var i int
	for d := range data {
		result[len(t.generic)+i] = d
		i++
	}
	return result
}

func longestPrefix(a, b []token) int {
	maxLen := len(a)
	if l := len(b); l < maxLen {
		maxLen = l
	}
	for i := range maxLen {
		if a[i] != b[i] {
			return i
		}
	}
	return maxLen
}

// traversalMarkers indicate traversal starting points in a URL.
var traversalMarkers byteset.Set

func init() {
	const markerChars = "-._~:/?#[]@!$&'()*+,;%="
	for i := range markerChars {
		ch := markerChars[i]
		traversalMarkers.Add(ch)
	}
}

func isTraversalMarker(char byte) bool {
	return traversalMarkers.Has(char)
}

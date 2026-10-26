package ruletree

import (
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/ruletree/byteset"
)

type Data comparable

// Tree is a prefix tree for storing and retrieving data
// associated with adblock-style patterns.
//
// Get may run concurrently with other Get calls. Insert and Compact must not
// run concurrently with any other method.
type Tree[T Data] struct {
	// domains and frozen store ||domain^ patterns, which make up the bulk of
	// filter lists, keyed by domain instead of as a per-character node chain.
	// domains takes new patterns, and Compact moves them into frozen. A map
	// entry costs about 70 bytes, mostly in slot overhead and a separate
	// allocation for each []T; a frozen entry costs 24.
	domains map[string][]T
	// frozen is sorted by key. A domain with several values repeats in
	// adjacent entries.
	frozen []domainEntry[T]

	root *node[T]
	// domainBoundaryRoot stores patterns beginning with domainBoundary (||).
	domainBoundaryRoot *node[T]
	// anchorRoot stores pattern beginning with tokenAnchor (|).
	anchorRoot *node[T]
}

type domainEntry[T Data] struct {
	key string
	val T
}

func New[T Data]() *Tree[T] {
	return &Tree[T]{
		domains:            make(map[string][]T),
		root:               &node[T]{},
		domainBoundaryRoot: &node[T]{},
		anchorRoot:         &node[T]{},
	}
}

// Insert adds a pattern with associated data to the tree.
func (t *Tree[T]) Insert(pattern string, v T) {
	if key, ok := domainKey(pattern); ok {
		t.domains[key] = append(t.domains[key], v)
		return
	}

	t.insertTree(pattern, v)
}

// insertTree adds a pattern to the prefix tree, bypassing the domain bucket.
func (t *Tree[T]) insertTree(pattern string, v T) {
	if pattern == "" {
		return
	}

	var parent *node[T]
	var n *node[T]

	switch {
	case strings.HasPrefix(pattern, domainBoundary):
		n, pattern = t.domainBoundaryRoot, pattern[len(domainBoundary):]
	case pattern[0] == tokenAnchor:
		n, pattern = t.anchorRoot, pattern[1:]
	default:
		n = t.root
	}
	if strings.Contains(pattern, domainBoundary) {
		return
	}

	tokens := tokenize(pattern)

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
			data[item] = struct{}{}
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
			t.matchDomainBoundary(url[i:], data)
		}
		if i > hostStart && (hostEnd == -1 || i < hostStart+hostEnd) {
			if c == '.' {
				t.matchDomainBoundary(url[i+1:], data)
			}
		}
	}

	result := make([]T, len(data))
	var i int
	for d := range data {
		result[i] = d
		i++
	}
	return result
}

// domainKey returns the domain of a pattern shaped exactly ||domain^, or false
// for anything else. For matchDomainBoundary to agree with the tree, the domain
// only has to be free of separators and of the tree's special characters. The
// accepted set is narrower on purpose: rarer shapes, such as a leading or
// trailing dot, stay in the tree because they aren't worth proving equivalent.
func domainKey(pattern string) (string, bool) {
	if len(pattern) < 4 || pattern[0] != '|' || pattern[1] != '|' || pattern[len(pattern)-1] != '^' {
		return "", false
	}
	domain := pattern[2 : len(pattern)-1]
	if domain[0] == '.' || domain[len(domain)-1] == '.' {
		return "", false
	}
	for i := 0; i < len(domain); i++ {
		c := domain[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '.' && c != '-' && c != '_' {
			return "", false
		}
	}
	return domain, true
}

// matchDomainBoundary adds the values of || patterns that match at the start
// of s to the into set, from both the tree and the domain bucket.
func (t *Tree[T]) matchDomainBoundary(s string, into map[T]struct{}) {
	for _, v := range t.domainBoundaryRoot.traverse(s) {
		into[v] = struct{}{}
	}

	// The tree matches ||domain^ here if s continues with a separator or ends
	// right after the domain, so the candidate is s up to its first separator.
	end := 0
	for end < len(s) && !isSeparator(s[end]) {
		end++
	}
	domain := s[:end]

	// A hand-written binary search: slices.BinarySearchFunc's comparison
	// callback made matching measurably slower.
	i, j := 0, len(t.frozen)
	for i < j {
		h := i + (j-i)/2
		if t.frozen[h].key < domain {
			i = h + 1
		} else {
			j = h
		}
	}
	for ; i < len(t.frozen) && t.frozen[i].key == domain; i++ {
		into[t.frozen[i].val] = struct{}{}
	}
	for _, v := range t.domains[domain] {
		into[v] = struct{}{}
	}
}

func longestPrefix(a, b string) int {
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

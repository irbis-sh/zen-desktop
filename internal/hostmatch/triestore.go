package hostmatch

import (
	"slices"
	"sort"
	"strings"
	"sync"
)

type node[T any] struct {
	// children is sorted by segment. Almost every node has one child or none,
	// and a one-entry slice is far smaller than a one-entry map.
	children []childEntry[T]
	data     []T
}

type childEntry[T any] struct {
	segment string
	node    *node[T]
}

func (n *node[T]) getChild(segment string) *node[T] {
	idx := n.searchChildren(segment)
	if idx < len(n.children) && n.children[idx].segment == segment {
		return n.children[idx].node
	}
	return nil
}

func (n *node[T]) findOrAddChild(segment string) *node[T] {
	idx := n.searchChildren(segment)
	if idx < len(n.children) && n.children[idx].segment == segment {
		return n.children[idx].node
	}

	newChild := &node[T]{}
	n.children = append(n.children, childEntry[T]{})
	copy(n.children[idx+1:], n.children[idx:])
	n.children[idx] = childEntry[T]{segment, newChild}
	return newChild
}

func (n *node[T]) searchChildren(segment string) int {
	return sort.Search(len(n.children), func(i int) bool {
		return n.children[i].segment >= segment
	})
}

func (n *node[T]) getMatchingData(segments []string, isWildcard bool) []T {
	if len(segments) == 0 {
		return n.data
	}

	var data []T
	if isWildcard {
		// Wildcards can consume as many segments as possible.
		data = append(data, n.getMatchingData(segments[1:], true)...)
	} else {
		// The hostname is a subdomain of the patterns ending here. Wildcard
		// nodes skip this: consuming the extra segments above already matches
		// their subdomains, and collecting here would count them twice.
		data = append(data, n.data...)
	}

	if wildcardChild := n.getChild("*"); wildcardChild != nil {
		data = append(data, wildcardChild.getMatchingData(segments[1:], true)...)
	}
	if exactChild := n.getChild(segments[0]); exactChild != nil {
		data = append(data, exactChild.getMatchingData(segments[1:], false)...)
	}

	return data
}

type trieStore[T any] struct {
	mu   sync.RWMutex
	root *node[T]
}

func newTrieStore[T any]() *trieStore[T] {
	return &trieStore[T]{
		root: &node[T]{},
	}
}

func (ts *trieStore[T]) Add(hostnamePattern string, data T) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	node := ts.root
	for _, segment := range reverseSegments(hostnamePattern) {
		node = node.findOrAddChild(segment)
	}
	node.data = append(node.data, data)
}

func (ts *trieStore[T]) Get(hostname string) []T {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return ts.root.getMatchingData(reverseSegments(hostname), false)
}

// reverseSegments splits a hostname into its dot-separated segments, last one
// first. Keying the trie from the TLD down puts every pattern a hostname is a
// subdomain of on its path: sub.example.com walks com → example → sub and
// collects the data for example.com on the way.
//
// Wildcards are unaffected: a * matches one or more whole segments wherever it
// sits, which holds equally when pattern and hostname are both reversed.
func reverseSegments(hostname string) []string {
	segments := strings.Split(hostname, ".")
	slices.Reverse(segments)
	return segments
}

package resolver

import (
	"container/list"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// maxCacheEntries bounds the cache. Expired entries are only removed when read, so this is
// also what bounds the ones that are never read again.
const maxCacheEntries = 10_000

// cacheKey identifies a cached answer. name is lower-cased, without the trailing dot.
type cacheKey struct {
	name  string
	qtype dnsmessage.Type
}

type cacheEntry struct {
	key     cacheKey
	verdict verdict
	expires time.Time
}

// cache is an LRU cache of DoH answers. A cache is safe for concurrent use.
type cache struct {
	mu      sync.Mutex
	entries map[cacheKey]*list.Element
	// order holds a *cacheEntry per entry, most recently used first.
	order *list.List
}

func newCache() *cache {
	return &cache{entries: make(map[cacheKey]*list.Element), order: list.New()}
}

// wallClock returns the time without its monotonic reading. The monotonic clock stops
// during sleep, and an entry's time has to run out while the machine sleeps too.
func wallClock() time.Time { return time.Now().Round(0) }

// get returns the fresh entry for key, if there is one.
func (c *cache) get(key cacheKey) (verdict, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[key]
	if !ok {
		return verdict{}, false
	}
	e := el.Value.(*cacheEntry)
	if !wallClock().Before(e.expires) {
		c.remove(el)
		return verdict{}, false
	}
	c.order.MoveToFront(el)
	return e.verdict, true
}

// put stores v under key for v.ttl.
func (c *cache) put(key cacheKey, v verdict) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[key]; ok {
		c.remove(el)
	}
	c.entries[key] = c.order.PushFront(&cacheEntry{key: key, verdict: v, expires: wallClock().Add(v.ttl)})
	if c.order.Len() > maxCacheEntries {
		c.remove(c.order.Back())
	}
}

func (c *cache) remove(el *list.Element) {
	c.order.Remove(el)
	delete(c.entries, el.Value.(*cacheEntry).key)
}

func (c *cache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	clear(c.entries)
	c.order.Init()
}

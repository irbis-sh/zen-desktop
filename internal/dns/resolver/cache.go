package resolver

import (
	"container/list"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// maxCacheEntries is the most entries the cache holds, expired ones included.
const maxCacheEntries = 10_000

// maxChainAge is how long an entry's chain stays readable after the entry expires. A
// host that the CNAME check blocks is never dialled, so nothing else would replace its
// entry: without a limit, a site that stopped using a tracker would stay blocked until the
// cache was cleared.
const maxChainAge = time.Hour

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

// wallClock returns the time without its monotonic reading. On macOS and Linux the
// monotonic clock stops during sleep, and an entry's time has to run out while the
// machine sleeps too.
func wallClock() time.Time { return time.Now().Round(0) }

// get returns the fresh entry for key, if there is one.
func (c *cache) get(key cacheKey) (verdict, bool) { return c.read(key, 0) }

// getStale is get that also returns an entry that expired less than maxChainAge ago.
// Like get, it counts as a use.
func (c *cache) getStale(key cacheKey) (verdict, bool) { return c.read(key, maxChainAge) }

// read returns the entry for key unless it expired grace ago or earlier, and counts that
// as a use.
func (c *cache) read(key cacheKey, grace time.Duration) (verdict, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[key]
	if !ok {
		return verdict{}, false
	}
	e := el.Value.(*cacheEntry)
	if !wallClock().Before(e.expires.Add(grace)) {
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

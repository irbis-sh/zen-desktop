package resolver

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestCacheEntryExpires(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := newCache()
		key := cacheKey{"news.example", dnsmessage.TypeA}
		c.put(key, verdict{ttl: 10 * time.Second})

		time.Sleep(10*time.Second - time.Millisecond)
		if _, ok := c.get(key); !ok {
			t.Error("entry gone before its TTL ran out")
		}
		time.Sleep(time.Millisecond)
		if _, ok := c.get(key); ok {
			t.Error("entry still there after its TTL ran out")
		}
		if len(c.entries) != 0 || c.order.Len() != 0 {
			t.Errorf("expired entry not removed: %d in map, %d in list", len(c.entries), c.order.Len())
		}
	})
}

func TestCacheKeysOnType(t *testing.T) {
	t.Parallel()

	c := newCache()
	c.put(cacheKey{"news.example", dnsmessage.TypeA}, verdict{ttl: time.Minute})

	if _, ok := c.get(cacheKey{"news.example", dnsmessage.TypeAAAA}); ok {
		t.Error("an A entry answered AAAA")
	}
}

func TestCachePutReplacesEntry(t *testing.T) {
	t.Parallel()

	c := newCache()
	key := cacheKey{"news.example", dnsmessage.TypeA}
	c.put(key, verdict{ttl: time.Minute})
	c.put(key, verdict{ttl: time.Minute, useOS: true})

	if v, _ := c.get(key); !v.useOS {
		t.Error("second put didn't replace the entry")
	}
	if c.order.Len() != 1 {
		t.Errorf("list holds %d entries, want 1", c.order.Len())
	}
}

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()

	c := newCache()
	keys := make([]cacheKey, maxCacheEntries)
	for i := range keys {
		keys[i] = cacheKey{fmt.Sprintf("%d.example", i), dnsmessage.TypeA}
		c.put(keys[i], verdict{ttl: time.Minute})
	}
	// Reading the oldest entry makes the second oldest the next to go.
	c.get(keys[0])
	c.put(cacheKey{"new.example", dnsmessage.TypeA}, verdict{ttl: time.Minute})

	if len(c.entries) != maxCacheEntries || c.order.Len() != maxCacheEntries {
		t.Errorf("cache holds %d in map, %d in list, want %d", len(c.entries), c.order.Len(), maxCacheEntries)
	}
	if _, ok := c.get(keys[0]); !ok {
		t.Error("evicted an entry that was just read")
	}
	if _, ok := c.get(keys[1]); ok {
		t.Error("least recently used entry is still there")
	}
}

func TestCacheClear(t *testing.T) {
	t.Parallel()

	c := newCache()
	key := cacheKey{"news.example", dnsmessage.TypeA}
	c.put(key, verdict{ttl: time.Minute})
	c.clear()

	if _, ok := c.get(key); ok {
		t.Error("entry survived clear")
	}
	if c.order.Len() != 0 {
		t.Errorf("list holds %d entries after clear", c.order.Len())
	}
}

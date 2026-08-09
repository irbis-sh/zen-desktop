package diskcache

import (
	"crypto/md5" // #nosec G501 -- MD5 is used to hash data, not for cryptographic purposes.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	indexFilename = "index.json"
	// defaultTTL is assumed for entries whose original TTL is unknown.
	defaultTTL = 24 * time.Hour
	// gcMaxAge is how long a stale entry may go unread before its content is
	// deleted. Contents are deliberately kept past their expiry so they can serve
	// as a fallback when fetching fails, so disk usage is bounded by access time,
	// not expiry. Fresh entries are never collected.
	gcMaxAge = 90 * 24 * time.Hour
	// lastAccessFlushInterval bounds how often Load persists its LastAccess bump:
	// often enough that GC never sees a regularly read entry as abandoned, rare
	// enough to avoid rewriting the index on every read.
	lastAccessFlushInterval = time.Hour
)

// Meta describes a cache entry.
type Meta struct {
	ExpiresAt    time.Time     `json:"expiresAt"`
	TTL          time.Duration `json:"ttl"`
	ETag         string        `json:"etag,omitempty"`
	LastModified string        `json:"lastModified,omitempty"`
}

// IsFresh reports whether the entry is within its expiry. Stale entries remain
// loadable: "expired" means "revalidate when possible", not "unusable".
func (m *Meta) IsFresh() bool {
	return time.Now().Before(m.ExpiresAt)
}

type urlHash string

type entry struct {
	Meta
	// LastAccess is the cache's private bookkeeping for garbage collection,
	// deliberately kept out of the [Meta] API surface.
	LastAccess time.Time `json:"lastAccess"`
}

type Cache struct {
	dir string

	entriesMu sync.Mutex
	entries   map[urlHash]*entry

	// flushMu serialises index writes: without it, a flush holding an older
	// snapshot could rename over one holding a newer snapshot.
	flushMu sync.Mutex

	// promoteMu makes the content rename and the index update in Promote one
	// atomic unit relative to other promotions: without it, two concurrent
	// promotions of the same URL could pair one response's bytes on disk with
	// the other response's metadata in the index.
	promoteMu sync.Mutex
}

var (
	cacheFileRegex = regexp.MustCompile(`^([0-9a-f]{32})\.cache\.txt$`)
	// legacyCacheFileRegex matches the pre-index naming scheme, which encoded
	// the expiry timestamp in the filename.
	legacyCacheFileRegex = regexp.MustCompile(`^([0-9a-f]{32})-(\d+)\.cache\.txt$`)
)

func New(cachePath string) (*Cache, error) {
	cache := &Cache{
		dir:     cachePath,
		entries: make(map[urlHash]*entry),
	}

	if err := cache.loadFromDisk(); err != nil {
		log.Printf("error loading cache from disk: %v", err)
	}

	return cache, nil
}

func (c *Cache) loadFromDisk() error {
	dirEntries, err := os.ReadDir(c.dir)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(c.dir, 0755); err != nil {
			return fmt.Errorf("create cache dir: %v", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("read cache dir: %v", err)
	}

	entries, err := readIndex(filepath.Join(c.dir, indexFilename))
	if err != nil {
		log.Printf("error reading cache index, rebuilding it: %v", err)
	}
	if entries == nil {
		entries = make(map[urlHash]*entry)
	}

	type legacyFile struct {
		name      string
		expiresAt int64
	}
	newestLegacy := make(map[urlHash]legacyFile)
	present := make(map[urlHash]struct{})

	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()

		if strings.HasSuffix(name, ".tmp") {
			// Leftover from a crash mid-download or mid-index-write.
			c.removeFile(name)
			continue
		}

		if matches := cacheFileRegex.FindStringSubmatch(name); matches != nil {
			present[urlHash(matches[1])] = struct{}{}
			continue
		}

		if matches := legacyCacheFileRegex.FindStringSubmatch(name); matches != nil {
			hash := urlHash(matches[1])
			timestamp, err := strconv.ParseInt(matches[2], 10, 64)
			if err != nil {
				continue
			}
			// The legacy Save never deleted a superseded file, so several may
			// exist per hash; keep only the newest.
			if prev, ok := newestLegacy[hash]; !ok || timestamp > prev.expiresAt {
				if ok {
					c.removeFile(prev.name)
				}
				newestLegacy[hash] = legacyFile{name: name, expiresAt: timestamp}
			} else {
				c.removeFile(name)
			}
		}
	}

	now := time.Now()

	// Migrate legacy files to the stable-name scheme. Expired ones are adopted
	// too: contents are kept past expiry as a fallback for failed fetches.
	for hash, lf := range newestLegacy {
		if _, ok := present[hash]; ok {
			c.removeFile(lf.name)
			continue
		}
		if err := renameWithRetry(filepath.Join(c.dir, lf.name), c.contentPath(hash)); err != nil {
			log.Printf("error migrating cache file %s: %v", lf.name, err)
			continue
		}
		present[hash] = struct{}{}
		if _, ok := entries[hash]; !ok {
			entries[hash] = &entry{
				Meta: Meta{
					ExpiresAt: time.Unix(lf.expiresAt, 0),
					TTL:       defaultTTL,
				},
				LastAccess: now,
			}
		}
	}

	// Reconcile index and directory in both directions: either side can be
	// missing entries after a crash between a content rename and an index write,
	// or after the OS or the user cleaned up parts of the cache directory.
	for hash := range entries {
		if _, ok := present[hash]; !ok {
			delete(entries, hash)
		}
	}
	for hash := range present {
		if _, ok := entries[hash]; !ok {
			// The zero ExpiresAt marks the entry as stale, so it gets
			// revalidated on first use but can still serve as a fallback.
			entries[hash] = &entry{
				Meta:       Meta{TTL: defaultTTL},
				LastAccess: now,
			}
		}
	}

	for hash, e := range entries {
		// GC targets abandoned entries; an unexpired entry is not abandoned,
		// no matter how stale its recorded access time is.
		if e.IsFresh() || now.Sub(e.LastAccess) <= gcMaxAge {
			continue
		}
		if err := os.Remove(c.contentPath(hash)); err != nil {
			log.Printf("error deleting unused cache file for %s: %v", hash, err)
			// The content is still on disk, so keep the entry.
			continue
		}
		delete(entries, hash)
	}

	c.entries = entries

	if err := c.Flush(); err != nil {
		log.Printf("error writing cache index: %v", err)
	}

	return nil
}

// Load returns the cached content and metadata for url, or (nil, nil, nil) when
// no usable entry exists. Stale entries are returned as well: the caller decides
// what freshness requires via Meta.IsFresh.
func (c *Cache) Load(url string) (io.ReadCloser, *Meta, error) {
	hash := hashURL(url)

	c.entriesMu.Lock()
	e, ok := c.entries[hash]
	if !ok {
		c.entriesMu.Unlock()
		return nil, nil, nil
	}
	now := time.Now()
	persistBump := now.Sub(e.LastAccess) > lastAccessFlushInterval
	e.LastAccess = now
	meta := e.Meta
	c.entriesMu.Unlock()

	if persistBump {
		// Persist the access-time bump so GC never mistakes a regularly read
		// entry for an abandoned one; the interval check bounds this to roughly
		// one index write per entry per hour, normally one per startup.
		if err := c.Flush(); err != nil {
			log.Printf("error writing cache index: %v", err)
		}
	}

	f, err := os.Open(c.contentPath(hash))
	if err != nil {
		if os.IsNotExist(err) {
			// The content is gone (OS cache purge, manual deletion): treat as
			// "no cache", but don't clobber an entry promoted since our lookup.
			c.entriesMu.Lock()
			if cur, ok := c.entries[hash]; ok && cur == e {
				delete(c.entries, hash)
			}
			c.entriesMu.Unlock()
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("open cache file: %w", err)
	}

	return f, &meta, nil
}

// Promote atomically installs tempPath (a fully written file inside the cache
// directory, see TempFile) as the content for url and records its metadata.
// On failure the previous entry is left intact and tempPath is removed.
func (c *Cache) Promote(url string, tempPath string, meta Meta) error {
	hash := hashURL(url)

	c.promoteMu.Lock()
	defer c.promoteMu.Unlock()

	if err := renameWithRetry(tempPath, c.contentPath(hash)); err != nil {
		if rmErr := os.Remove(tempPath); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Printf("error removing temp file %s: %v", tempPath, rmErr)
		}
		return fmt.Errorf("install cache file: %w", err)
	}

	c.entriesMu.Lock()
	c.entries[hash] = &entry{
		Meta:       meta,
		LastAccess: time.Now(),
	}
	c.entriesMu.Unlock()

	if err := c.Flush(); err != nil {
		log.Printf("error writing cache index: %v", err)
	}

	return nil
}

// Refresh extends the expiry of an existing entry without rewriting its content,
// e.g. after a 304 response. Empty validators keep the stored ones.
func (c *Cache) Refresh(url string, expiresAt time.Time, etag, lastModified string) {
	hash := hashURL(url)

	c.entriesMu.Lock()
	e, ok := c.entries[hash]
	if !ok {
		c.entriesMu.Unlock()
		return
	}
	e.ExpiresAt = expiresAt
	if etag != "" {
		e.ETag = etag
	}
	if lastModified != "" {
		e.LastModified = lastModified
	}
	e.LastAccess = time.Now()
	c.entriesMu.Unlock()

	if err := c.Flush(); err != nil {
		log.Printf("error writing cache index: %v", err)
	}
}

// TempFile creates a temporary file inside the cache directory, suitable for
// writing content that will be handed to Promote. Living on the same filesystem
// as the cache keeps the promoting rename atomic; leftovers from crashes are
// swept at startup by their .tmp suffix.
func (c *Cache) TempFile() (*os.File, error) {
	return os.CreateTemp(c.dir, "download-*.tmp")
}

// Flush atomically persists the index to disk.
func (c *Cache) Flush() error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	c.entriesMu.Lock()
	data, err := json.Marshal(c.entries)
	c.entriesMu.Unlock()
	if err != nil {
		return fmt.Errorf("marshal index: %v", err)
	}

	tmp, err := os.CreateTemp(c.dir, "index-*.tmp")
	if err != nil {
		return fmt.Errorf("create index temp file: %v", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write index: %v", err)
	}
	// The rename below is atomic for the name but not the data: without a
	// sync, a crash shortly after could persist an empty or truncated index.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("sync index: %v", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("close index: %v", err)
	}
	if err := renameWithRetry(tmp.Name(), filepath.Join(c.dir, indexFilename)); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("rename index: %v", err)
	}

	return nil
}

func (c *Cache) contentPath(hash urlHash) string {
	return filepath.Join(c.dir, string(hash)+".cache.txt")
}

func (c *Cache) removeFile(name string) {
	if err := os.Remove(filepath.Join(c.dir, name)); err != nil {
		log.Printf("error removing cache file %s: %v", name, err)
	}
}

func readIndex(path string) (map[urlHash]*entry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var entries map[urlHash]*entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	// A JSON null unmarshals to a nil *entry, which every consumer would
	// dereference. Drop such entries; their content files, if any, get adopted
	// as stale by the reconciliation pass.
	for hash, e := range entries {
		if e == nil {
			delete(entries, hash)
		}
	}
	return entries, nil
}

// renameWithRetry retries a failed rename a few times to ride out transient
// sharing violations on Windows, where antivirus scanners or a concurrent
// reader can briefly hold the destination open.
func renameWithRetry(from, to string) error {
	var err error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		// A missing source is not a sharing violation: retrying cannot make
		// the file appear, so fail fast.
		if err = os.Rename(from, to); err == nil || os.IsNotExist(err) {
			return err
		}
	}
	return err
}

func hashURL(url string) urlHash {
	sum := md5.Sum([]byte(url)) // #nosec G401
	return urlHash(hex.EncodeToString(sum[:]))
}

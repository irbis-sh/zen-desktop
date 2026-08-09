package diskcache

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testURL = "https://example.com/list.txt"

func TestFreshEntryRoundTrip(t *testing.T) {
	t.Parallel()

	c := newTestCache(t, t.TempDir())
	promoteString(t, c, "rules-v1", Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: time.Hour})

	content, meta := loadString(t, c, testURL)
	if content != "rules-v1" {
		t.Errorf("content = %q, want %q", content, "rules-v1")
	}
	if meta == nil || !meta.IsFresh() {
		t.Errorf("meta = %+v, want fresh", meta)
	}
}

func TestStaleEntrySurvivesRestart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := newTestCache(t, dir)
	promoteString(t, c, "rules-v1", Meta{ExpiresAt: time.Now().Add(-time.Hour), TTL: time.Hour})

	c2 := newTestCache(t, dir)
	content, meta := loadString(t, c2, testURL)
	if content != "rules-v1" {
		t.Errorf("content = %q, want %q", content, "rules-v1")
	}
	if meta == nil {
		t.Fatal("meta is nil, want stale meta")
	}
	if meta.IsFresh() {
		t.Error("meta.IsFresh() = true, want false")
	}
	if meta.TTL != time.Hour {
		t.Errorf("meta.TTL = %v, want %v", meta.TTL, time.Hour)
	}
}

func TestLegacyMigrationKeepsNewest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hash := string(hashURL(testURL))
	// Both timestamps are in the past: the legacy format encoded the expiry in
	// the filename, and expired entries must be adopted rather than deleted.
	if err := os.WriteFile(filepath.Join(dir, hash+"-1000000000.cache.txt"), []byte("older"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, hash+"-1100000000.cache.txt"), []byte("newer"), 0644); err != nil {
		t.Fatal(err)
	}

	c := newTestCache(t, dir)

	content, meta := loadString(t, c, testURL)
	if content != "newer" {
		t.Errorf("content = %q, want %q", content, "newer")
	}
	if meta == nil {
		t.Fatal("meta is nil, want adopted entry")
	}
	if meta.IsFresh() {
		t.Error("meta.IsFresh() = true, want false for an expired legacy entry")
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range dirEntries {
		if legacyCacheFileRegex.MatchString(e.Name()) {
			t.Errorf("legacy file %s survived migration", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, contentFilename(testURL))); err != nil {
		t.Errorf("migrated content file missing: %v", err)
	}
}

func TestLegacyMigrationPreservesExpiry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hash := string(hashURL(testURL))
	future := time.Now().Add(time.Hour).Unix()
	if err := os.WriteFile(filepath.Join(dir, hash+"-"+strconv.FormatInt(future, 10)+".cache.txt"), []byte("rules"), 0644); err != nil {
		t.Fatal(err)
	}

	c := newTestCache(t, dir)
	content, meta := loadString(t, c, testURL)
	if content != "rules" {
		t.Errorf("content = %q, want %q", content, "rules")
	}
	if meta == nil || !meta.IsFresh() {
		t.Errorf("meta = %+v, want fresh (legacy expiry in the future)", meta)
	}
}

func TestOrphanContentAdopted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, contentFilename(testURL)), []byte("orphan"), 0644); err != nil {
		t.Fatal(err)
	}

	c := newTestCache(t, dir)
	content, meta := loadString(t, c, testURL)
	if content != "orphan" {
		t.Errorf("content = %q, want %q", content, "orphan")
	}
	if meta == nil {
		t.Fatal("meta is nil, want adopted entry")
	}
	if meta.IsFresh() {
		t.Error("adopted orphan should be stale")
	}
}

func TestIndexEntryWithoutContentDropped(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeIndex(t, dir, map[urlHash]*entry{
		hashURL(testURL): {Meta: Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: time.Hour}, LastAccess: time.Now()},
	})

	c := newTestCache(t, dir)
	if content, meta := loadString(t, c, testURL); meta != nil {
		t.Errorf("Load returned (%q, %+v), want no entry", content, meta)
	}

	entries, err := readIndex(filepath.Join(dir, indexFilename))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("index has %d entries after reconciliation, want 0", len(entries))
	}
}

func TestMissingContentTreatedAsNoCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := newTestCache(t, dir)
	promoteString(t, c, "rules", Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: time.Hour})

	if err := os.Remove(filepath.Join(dir, contentFilename(testURL))); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		rc, meta, err := c.Load(testURL)
		if err != nil {
			t.Fatalf("Load: %v, want no error for missing content", err)
		}
		if rc != nil || meta != nil {
			t.Fatalf("Load = (%v, %+v), want (nil, nil)", rc, meta)
		}
	}
}

func TestPromoteFailureKeepsPreviousEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := newTestCache(t, dir)
	expiresAt := time.Now().Add(time.Hour)
	promoteString(t, c, "rules-v1", Meta{ExpiresAt: expiresAt, TTL: time.Hour})

	err := c.Promote(testURL, filepath.Join(dir, "nonexistent.tmp"), Meta{ExpiresAt: time.Now().Add(2 * time.Hour), TTL: time.Hour})
	if err == nil {
		t.Fatal("Promote with a missing temp file succeeded, want error")
	}

	content, meta := loadString(t, c, testURL)
	if content != "rules-v1" {
		t.Errorf("content = %q, want previous %q", content, "rules-v1")
	}
	if meta == nil || !meta.ExpiresAt.Equal(expiresAt) {
		t.Errorf("meta = %+v, want previous expiry %v", meta, expiresAt)
	}
}

func TestPromoteLeavesNoTempFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := newTestCache(t, dir)
	promoteString(t, c, "rules", Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: time.Hour})

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range dirEntries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file %s left behind", e.Name())
		}
	}
}

func TestRefreshPersists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := newTestCache(t, dir)
	promoteString(t, c, "rules", Meta{ExpiresAt: time.Now().Add(-time.Hour), TTL: time.Hour})

	newExpiry := time.Now().Add(2 * time.Hour)
	c.Refresh(testURL, newExpiry, `"etag-1"`, "Mon, 02 Jan 2006 15:04:05 GMT")

	c2 := newTestCache(t, dir)
	content, meta := loadString(t, c2, testURL)
	if content != "rules" {
		t.Errorf("content = %q, want %q", content, "rules")
	}
	if meta == nil {
		t.Fatal("meta is nil")
	}
	if !meta.IsFresh() {
		t.Error("meta.IsFresh() = false after Refresh, want true")
	}
	if meta.ETag != `"etag-1"` {
		t.Errorf("meta.ETag = %q, want %q", meta.ETag, `"etag-1"`)
	}
}

func TestGCDeletesLongUnaccessedEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	const recentURL = "https://example.com/recent.txt"
	if err := os.WriteFile(filepath.Join(dir, contentFilename(testURL)), []byte("ancient"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, contentFilename(recentURL)), []byte("recent"), 0644); err != nil {
		t.Fatal(err)
	}
	writeIndex(t, dir, map[urlHash]*entry{
		hashURL(testURL):   {Meta: Meta{ExpiresAt: time.Now(), TTL: time.Hour}, LastAccess: time.Now().Add(-gcMaxAge - time.Hour)},
		hashURL(recentURL): {Meta: Meta{ExpiresAt: time.Now(), TTL: time.Hour}, LastAccess: time.Now()},
	})

	c := newTestCache(t, dir)

	if content, meta := loadString(t, c, testURL); meta != nil {
		t.Errorf("long-unaccessed entry survived GC: (%q, %+v)", content, meta)
	}
	if _, err := os.Stat(filepath.Join(dir, contentFilename(testURL))); !os.IsNotExist(err) {
		t.Errorf("long-unaccessed content file survived GC: %v", err)
	}
	if content, _ := loadString(t, c, recentURL); content != "recent" {
		t.Errorf("recently accessed entry did not survive GC: %q", content)
	}
}

func TestLoadDoesNotRewriteIndex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := newTestCache(t, dir)
	promoteString(t, c, "rules", Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: time.Hour})

	if err := os.Remove(filepath.Join(dir, indexFilename)); err != nil {
		t.Fatal(err)
	}

	if content, _ := loadString(t, c, testURL); content != "rules" {
		t.Fatalf("content = %q, want %q", content, "rules")
	}

	if _, err := os.Stat(filepath.Join(dir, indexFilename)); !os.IsNotExist(err) {
		t.Errorf("Load rewrote the index: stat err = %v, want not-exist", err)
	}
}

func TestTempFilesSweptAtStartup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "download-123.tmp"), []byte("partial download"), 0644); err != nil {
		t.Fatal(err)
	}

	newTestCache(t, dir)

	if _, err := os.Stat(filepath.Join(dir, "download-123.tmp")); !os.IsNotExist(err) {
		t.Errorf("leftover temp file survived startup sweep: %v", err)
	}
}

func TestCorruptIndexRebuilt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, indexFilename), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, contentFilename(testURL)), []byte("rules"), 0644); err != nil {
		t.Fatal(err)
	}

	c := newTestCache(t, dir)
	content, meta := loadString(t, c, testURL)
	if content != "rules" || meta == nil {
		t.Errorf("Load = (%q, %+v), want adopted entry despite corrupt index", content, meta)
	}
}

func TestNullIndexEntryTolerated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hash := string(hashURL(testURL))
	if err := os.WriteFile(filepath.Join(dir, indexFilename), []byte(`{"`+hash+`": null}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, contentFilename(testURL)), []byte("rules"), 0644); err != nil {
		t.Fatal(err)
	}

	c := newTestCache(t, dir)
	content, meta := loadString(t, c, testURL)
	if content != "rules" {
		t.Errorf("content = %q, want %q", content, "rules")
	}
	if meta == nil {
		t.Fatal("meta is nil, want the content adopted as a stale entry")
	}
	if meta.IsFresh() {
		t.Error("entry adopted from a null index value should be stale")
	}
}

func TestLoadPersistsStaleAccessBump(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, contentFilename(testURL)), []byte("rules"), 0644); err != nil {
		t.Fatal(err)
	}
	oldAccess := time.Now().Add(-2 * lastAccessFlushInterval)
	writeIndex(t, dir, map[urlHash]*entry{
		hashURL(testURL): {Meta: Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: time.Hour}, LastAccess: oldAccess},
	})

	c := newTestCache(t, dir)
	if content, _ := loadString(t, c, testURL); content != "rules" {
		t.Fatalf("content = %q, want %q", content, "rules")
	}

	entries, err := readIndex(filepath.Join(dir, indexFilename))
	if err != nil {
		t.Fatal(err)
	}
	e, ok := entries[hashURL(testURL)]
	if !ok {
		t.Fatal("entry missing from persisted index")
	}
	if !e.LastAccess.After(oldAccess) {
		t.Errorf("persisted LastAccess = %v, want bumped past %v", e.LastAccess, oldAccess)
	}
}

func TestGCKeepsFreshEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, contentFilename(testURL)), []byte("rules"), 0644); err != nil {
		t.Fatal(err)
	}
	writeIndex(t, dir, map[urlHash]*entry{
		hashURL(testURL): {
			Meta:       Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: 200 * 24 * time.Hour},
			LastAccess: time.Now().Add(-gcMaxAge - time.Hour),
		},
	})

	c := newTestCache(t, dir)
	if content, _ := loadString(t, c, testURL); content != "rules" {
		t.Errorf("fresh entry with old LastAccess did not survive GC: %q", content)
	}
}

func TestPromoteFailureRemovesTempFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := newTestCache(t, dir)

	// A directory at the destination path makes every rename attempt fail.
	if err := os.Mkdir(filepath.Join(dir, contentFilename(testURL)), 0755); err != nil {
		t.Fatal(err)
	}

	tmp, err := c.TempFile()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString("content"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	if err := c.Promote(testURL, tmp.Name(), Meta{ExpiresAt: time.Now().Add(time.Hour), TTL: time.Hour}); err == nil {
		t.Fatal("Promote succeeded with a directory at the destination, want error")
	}

	if _, err := os.Stat(tmp.Name()); !os.IsNotExist(err) {
		t.Errorf("temp file not removed after failed Promote: stat err = %v", err)
	}
	if rc, meta, err := c.Load(testURL); err != nil || rc != nil || meta != nil {
		t.Errorf("Load after failed Promote = (%v, %+v, %v), want no entry", rc, meta, err)
	}
}

func newTestCache(t *testing.T, dir string) *Cache {
	t.Helper()
	c, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func promoteString(t *testing.T, c *Cache, content string, meta Meta) {
	t.Helper()
	tmp, err := c.TempFile()
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	if err := c.Promote(testURL, tmp.Name(), meta); err != nil {
		t.Fatalf("Promote: %v", err)
	}
}

// loadString returns the content and metadata for url, or ("", nil) when the
// cache has no entry.
func loadString(t *testing.T, c *Cache, url string) (string, *Meta) {
	t.Helper()
	rc, meta, err := c.Load(url)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rc == nil {
		return "", nil
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read cache content: %v", err)
	}
	return string(b), meta
}

func contentFilename(url string) string {
	return string(hashURL(url)) + ".cache.txt"
}

func writeIndex(t *testing.T, dir string, entries map[urlHash]*entry) {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, indexFilename), data, 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}
}

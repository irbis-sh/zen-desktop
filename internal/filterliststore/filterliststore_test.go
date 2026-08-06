package filterliststore

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/filterliststore/diskcache"
)

const testListContent = "! Title: Test List\n! Expires: 12 hours\n||example.com^\n||ads.example.net^\n"

func TestFetchStreamsAndCaches(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		io.WriteString(w, testListContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	store := newTestStore(t, dir)

	content := getAndReadAll(t, store, server.URL, ModeDefault)
	if content != testListContent {
		t.Fatalf("got content %q, want %q", content, testListContent)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("got %d requests, want 1", got)
	}

	if got, want := cachedTTL(t, store, server.URL), 12*time.Hour; got != want {
		t.Errorf("got cached TTL %v, want %v (\"! Expires\" not honoured)", got, want)
	}

	// A second Get must be served from the fresh cache without a request.
	content = getAndReadAll(t, store, server.URL, ModeDefault)
	if content != testListContent {
		t.Fatalf("got cached content %q, want %q", content, testListContent)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests after cached Get, want 1", got)
	}

	assertNoTempFiles(t, dir)
}

func TestMidBodyDropSurfacesError(t *testing.T) {
	t.Parallel()

	const partial = "||example.com^\n||ads.exampl"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Hijack to declare more content than gets sent, then close the
		// connection cleanly: the client deterministically observes a short
		// length-delimited body.
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		fmt.Fprintf(buf, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(partial)*2, partial)
		buf.Flush()
	}))
	defer server.Close()

	dir := t.TempDir()
	store := newTestStore(t, dir)

	reader, err := store.Get(server.URL, ModeDefault)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_, err = io.ReadAll(reader)
	reader.Close()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got read error %v, want io.ErrUnexpectedEOF", err)
	}

	assertNotCached(t, store, server.URL)
	assertNoTempFiles(t, dir)
}

// TestIncompleteBodySurfacesStoreError drives cachingReader directly: the
// store's own completeness check cannot be reached through httptest, because
// Go's transport turns every short length-delimited body into
// io.ErrUnexpectedEOF before the reader ever sees a clean EOF. The check
// exists as defence in depth against transports without that guarantee.
func TestIncompleteBodySurfacesStoreError(t *testing.T) {
	t.Parallel()

	const url = "https://filters.example.com/list.txt"
	dir := t.TempDir()
	store := newTestStore(t, dir)

	tempFile, err := store.cache.TempFile()
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	reader := &cachingReader{
		body:          io.NopCloser(strings.NewReader("||example.com^\n")),
		tempFile:      tempFile,
		contentLength: 100,
		onComplete: func(*os.File) {
			t.Error("onComplete called for an incomplete body")
		},
	}

	_, err = io.ReadAll(reader)
	reader.Close()
	if !errors.Is(err, errIncompleteBody) {
		t.Fatalf("got read error %v, want errIncompleteBody", err)
	}
	// Both truncation forms must be matchable with one errors.Is check.
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("errIncompleteBody does not wrap io.ErrUnexpectedEOF")
	}

	assertNotCached(t, store, url)
	assertNoTempFiles(t, dir)
}

func TestEmptyBodyNotCached(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()

	dir := t.TempDir()
	store := newTestStore(t, dir)

	reader, err := store.Get(server.URL, ModeDefault)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_, err = io.ReadAll(reader)
	reader.Close()
	if !errors.Is(err, errEmptyBody) {
		t.Fatalf("got read error %v, want errEmptyBody", err)
	}

	assertNotCached(t, store, server.URL)
	assertNoTempFiles(t, dir)
}

func TestCallerEarlyCloseAbandonsDownload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "! Title: Big List\n")
		io.WriteString(w, strings.Repeat("||example.com^\n", 1<<16))
	}))
	defer server.Close()

	dir := t.TempDir()
	store := newTestStore(t, dir)

	reader, err := store.Get(server.URL, ModeDefault)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := reader.Read(make([]byte, 10)); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close before EOF returned an error: %v", err)
	}

	assertNotCached(t, store, server.URL)
	assertNoTempFiles(t, dir)
}

func TestHTMLResponseNotCached(t *testing.T) {
	t.Parallel()

	for _, contentType := range []string{
		"text/html; charset=utf-8",
		// A malformed parameter list must not defeat the guard: ParseMediaType
		// reports ErrInvalidMediaParameter but still yields the media type.
		"text/html; ; charset=utf-8",
	} {
		t.Run(contentType, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentType)
				io.WriteString(w, "<html><body>Sign in to continue</body></html>")
			}))
			defer server.Close()

			dir := t.TempDir()
			store := newTestStore(t, dir)

			if _, err := store.Get(server.URL, ModeDefault); err == nil {
				t.Fatalf("Get accepted a %q response", contentType)
			}

			assertNotCached(t, store, server.URL)
			assertNoTempFiles(t, dir)
		})
	}
}

func TestCacheOnlyMissNeverDials(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())

	if _, err := store.Get(server.URL, ModeCacheOnly); err == nil {
		t.Fatal("ModeCacheOnly with no cache entry succeeded")
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("got %d requests, want 0", got)
	}
}

func TestStaleServedByMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		mode FetchMode
	}{
		{"PreferCache", ModePreferCache},
		{"CacheOnly", ModeCacheOnly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
			}))
			defer server.Close()

			const stale = "||stale.example.com^\n"
			store := newTestStore(t, t.TempDir())
			seedStaleEntry(t, store, server.URL, stale)

			if content := getAndReadAll(t, store, server.URL, tc.mode); content != stale {
				t.Errorf("got content %q, want %q", content, stale)
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("got %d requests, want 0", got)
			}
		})
	}
}

func TestDefaultModeRefetchesStale(t *testing.T) {
	t.Parallel()

	const fresh = "||fresh.example.com^\n"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		io.WriteString(w, fresh)
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())
	seedStaleEntry(t, store, server.URL, "||stale.example.com^\n")

	if content := getAndReadAll(t, store, server.URL, ModeDefault); content != fresh {
		t.Errorf("got content %q, want refetched %q", content, fresh)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests, want 1", got)
	}
}

func newTestStore(t *testing.T, dir string) *FilterListStore {
	t.Helper()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func getAndReadAll(t *testing.T, store *FilterListStore, url string, mode FetchMode) string {
	t.Helper()
	reader, err := store.Get(url, mode)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read filter list: %v", err)
	}
	return string(content)
}

// seedStaleEntry installs an already-expired cache entry for url through the
// cache's own API, so the tests carry no knowledge of its on-disk format.
func seedStaleEntry(t *testing.T, store *FilterListStore, url, content string) {
	t.Helper()
	tempFile, err := store.cache.TempFile()
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	if _, err := tempFile.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	if err := store.cache.Promote(url, tempFile.Name(), diskcache.Meta{
		ExpiresAt: time.Now().Add(-time.Hour),
		TTL:       24 * time.Hour,
	}); err != nil {
		t.Fatalf("Promote: %v", err)
	}
}

func cachedTTL(t *testing.T, store *FilterListStore, url string) time.Duration {
	t.Helper()
	content, meta, err := store.cache.Load(url)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if content == nil {
		t.Fatalf("no cache entry for %q", url)
	}
	content.Close()
	return meta.TTL
}

func assertNotCached(t *testing.T, store *FilterListStore, url string) {
	t.Helper()
	if content, _, err := store.cache.Load(url); err != nil {
		t.Fatalf("Load: %v", err)
	} else if content != nil {
		content.Close()
		t.Errorf("%q unexpectedly present in the cache", url)
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(leftovers) > 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

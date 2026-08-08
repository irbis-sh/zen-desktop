package filterliststore

import (
	"context"
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

const (
	testListContent = "! Title: Test List\n! Expires: 12 hours\n||example.com^\n||ads.example.net^\n"
	// staleListContent is what seedStaleEntry installs.
	staleListContent = "||stale.example.com^\n"
)

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

	reader, _, err := store.Get(t.Context(), server.URL, ModeDefault)
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

	reader, _, err := store.Get(t.Context(), server.URL, ModeDefault)
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

// TestOversizedBodyRejected drives cachingReader directly, like
// TestIncompleteBodySurfacesStoreError: the real cap is far more data than a
// unit test should shuttle, so a small maxSize stands in for it.
func TestOversizedBodyRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newTestStore(t, dir)

	tempFile, err := store.cache.TempFile()
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	reader := &cachingReader{
		body:          io.NopCloser(strings.NewReader(strings.Repeat("a", 100))),
		tempFile:      tempFile,
		contentLength: -1,
		maxSize:       50,
		onComplete: func(*os.File) {
			t.Error("onComplete called for an oversized body")
		},
	}

	_, err = io.ReadAll(reader)
	reader.Close()
	if !errors.Is(err, errListTooLarge) {
		t.Fatalf("got read error %v, want errListTooLarge", err)
	}
	assertNoTempFiles(t, dir)
}

// TestReadIntoMemoryRejectsOversizedContent pins the cap on the in-memory
// serve path: an oversized cache file must be refused, not allocated.
func TestReadIntoMemoryRejectsOversizedContent(t *testing.T) {
	t.Parallel()

	if _, err := readIntoMemory(io.NopCloser(strings.NewReader("0123456789")), 5); !errors.Is(err, errListTooLarge) {
		t.Fatalf("got %v, want errListTooLarge", err)
	}

	content, err := readIntoMemory(io.NopCloser(strings.NewReader("0123456789")), 10)
	if err != nil {
		t.Fatalf("readIntoMemory within the limit: %v", err)
	}
	data, err := io.ReadAll(content)
	if err != nil || string(data) != "0123456789" {
		t.Fatalf("got (%q, %v), want the full content", data, err)
	}
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

	reader, _, err := store.Get(t.Context(), server.URL, ModeDefault)
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

			if _, _, err := store.Get(t.Context(), server.URL, ModeDefault); err == nil {
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

	if _, _, err := store.Get(t.Context(), server.URL, ModeCacheOnly); err == nil {
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

			store := newTestStore(t, t.TempDir())
			seedStaleEntry(t, store, server.URL)

			if content := getAndReadAll(t, store, server.URL, tc.mode); content != staleListContent {
				t.Errorf("got content %q, want %q", content, staleListContent)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// The stale entry stores no validators, so there is nothing to
		// condition the request on.
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			t.Error("conditional headers sent without stored validators")
		}
		io.WriteString(w, fresh)
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())
	seedStaleEntry(t, store, server.URL)

	if content := getAndReadAll(t, store, server.URL, ModeDefault); content != fresh {
		t.Errorf("got content %q, want refetched %q", content, fresh)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests, want 1", got)
	}
}

func TestValidatorsCapturedAndRevalidated(t *testing.T) {
	t.Parallel()

	const etag = `"v1"`
	const lastModified = "Wed, 21 Oct 2015 07:28:00 GMT"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("ETag", etag)
			w.Header().Set("Last-Modified", lastModified)
			io.WriteString(w, testListContent)
			return
		}
		if got := r.Header.Get("If-None-Match"); got != etag {
			t.Errorf("got If-None-Match %q, want %q", got, etag)
		}
		if got := r.Header.Get("If-Modified-Since"); got != lastModified {
			t.Errorf("got If-Modified-Since %q, want %q", got, lastModified)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())

	if content := getAndReadAll(t, store, server.URL, ModeDefault); content != testListContent {
		t.Fatalf("got content %q, want %q", content, testListContent)
	}

	// Expire the entry without touching its content or validators, so the
	// next Get has to revalidate.
	store.cache.Refresh(server.URL, time.Now().Add(-time.Hour), "", "")

	content, src := getAndReadAllSource(t, store, server.URL, ModeDefault)
	if content != testListContent {
		t.Errorf("got content %q, want cached %q", content, testListContent)
	}
	if src != SourceCache {
		t.Errorf("got source %v, want SourceCache (revalidated content is fresh)", src)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("got %d requests, want 2", got)
	}

	// The 304 must have bumped the expiry by the stored TTL: another Get is a
	// plain cache hit, without a request.
	if content := getAndReadAll(t, store, server.URL, ModeDefault); content != testListContent {
		t.Errorf("got content %q, want %q", content, testListContent)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("got %d requests after revalidation, want 2 (expiry not refreshed)", got)
	}
	assertSlotsFree(t, store)
}

// TestUnsolicited304NotTreatedAsFresh covers a 304 answering a request that
// carried no validators - a protocol violation seen from misbehaving servers
// and middleboxes. It confirms nothing, so it must be handled as a failed
// fetch: stale copy served, expiry left alone.
func TestUnsolicited304NotTreatedAsFresh(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			t.Error("conditional headers sent without stored validators")
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	store := newFastRetryStore(t, t.TempDir())
	seedStaleEntry(t, store, server.URL)

	content, src := getAndReadAllSource(t, store, server.URL, ModeDefault)
	if content != staleListContent {
		t.Errorf("got content %q, want %q", content, staleListContent)
	}
	if src != SourceStaleCache {
		t.Errorf("got source %v, want SourceStaleCache", src)
	}

	// The entry must still be stale: a second Get fetches again instead of
	// serving a bogusly refreshed copy.
	getAndReadAll(t, store, server.URL, ModeDefault)
	if got := requests.Load(); got != 2 {
		t.Errorf("got %d requests, want 2 (rogue 304 refreshed the entry)", got)
	}
	assertSlotsFree(t, store)
}

// TestMissingContentRefetchedUnconditionally covers an entry that still has
// validators while its content file is gone (OS cache purge, manual cleanup).
// Confirming freshness for content the store cannot produce would serve
// nothing, so the request must go out unconditional.
func TestMissingContentRefetchedUnconditionally(t *testing.T) {
	t.Parallel()

	const fresh = "||fresh.example.com^\n"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			t.Error("conditional request for content the store cannot produce")
		}
		io.WriteString(w, fresh)
	}))
	defer server.Close()

	dir := t.TempDir()
	store := newTestStore(t, dir)
	seedStaleEntry(t, store, server.URL)
	store.cache.Refresh(server.URL, time.Now().Add(-time.Hour), `"v1"`, "Wed, 21 Oct 2015 07:28:00 GMT")
	removeCacheContent(t, dir)

	content, src := getAndReadAllSource(t, store, server.URL, ModeDefault)
	if content != fresh {
		t.Errorf("got content %q, want refetched %q", content, fresh)
	}
	if src != SourceNetwork {
		t.Errorf("got source %v, want SourceNetwork", src)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests, want 1", got)
	}
}

// TestCancelledFetchNotMaskedByStaleCopy: a Get that fails because the
// caller's own context ended must report that, not dress the failure up as a
// successful stale serve - that would defeat the deadline the caller is
// enforcing.
func TestCancelledFetchNotMaskedByStaleCopy(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// Hang until the client gives up.
		<-r.Context().Done()
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())
	seedStaleEntry(t, store, server.URL)

	ctx, cancel := context.WithCancel(t.Context())
	type outcome struct {
		src Source
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		_, src, err := store.Get(ctx, server.URL, ModeDefault)
		done <- outcome{src: src, err: err}
	}()
	waitFor(t, func() bool { return requests.Load() == 1 })
	cancel()

	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("got (source %v, err %v), want context.Canceled", got.src, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Get never returned after cancellation")
	}

	store.flightMu.Lock()
	pending := len(store.inflight)
	store.flightMu.Unlock()
	if pending != 0 {
		t.Errorf("%d flights left behind", pending)
	}
	assertSlotsFree(t, store)
}

// TestCacheOnlyContentServedFromMemory proves cache-only serves are read into
// memory up front: the returned reader keeps working even when the content
// file disappears mid-parse. The rebuild loop's final cache-only pass counts
// on a Get that succeeded being unable to taint the parse.
func TestCacheOnlyContentServedFromMemory(t *testing.T) {
	t.Parallel()

	const url = "https://filters.example.com/list.txt"
	dir := t.TempDir()
	store := newTestStore(t, dir)
	seedStaleEntry(t, store, url)

	reader, src, err := store.Get(t.Context(), url, ModeCacheOnly)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()
	if src != SourceStaleCache {
		t.Errorf("got source %v, want SourceStaleCache", src)
	}

	// The file vanishing after Get must not affect the read.
	removeCacheContent(t, dir)

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read after content file removal: %v", err)
	}
	if string(content) != staleListContent {
		t.Errorf("got content %q, want %q", content, staleListContent)
	}
}

func TestSourceReflectsOrigin(t *testing.T) {
	t.Parallel()

	var fail atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, testListContent)
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())

	assertSource := func(mode FetchMode, want Source) {
		t.Helper()
		if _, src := getAndReadAllSource(t, store, server.URL, mode); src != want {
			t.Errorf("got source %v, want %v", src, want)
		}
	}

	assertSource(ModeDefault, SourceNetwork)
	assertSource(ModeDefault, SourceCache)

	// Expire the entry; a mode that tolerates staleness serves it as-is.
	store.cache.Refresh(server.URL, time.Now().Add(-time.Hour), "", "")
	assertSource(ModePreferCache, SourceStaleCache)

	// And so does a failed refetch.
	fail.Store(true)
	assertSource(ModeDefault, SourceStaleCache)

	if got := requests.Load(); got != 2 {
		t.Errorf("got %d requests, want 2", got)
	}
	assertSlotsFree(t, store)
}

func TestSlowButMovingBodySucceeds(t *testing.T) {
	t.Parallel()

	const line = "||example.com^\n"
	const chunks = 6
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		for range chunks {
			// Each gap is well under the watchdog budget: slow but alive.
			time.Sleep(60 * time.Millisecond)
			io.WriteString(w, line)
			flusher.Flush()
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	store := newTestStore(t, dir)
	store.stallTimeout = 250 * time.Millisecond

	want := strings.Repeat(line, chunks)
	if content := getAndReadAll(t, store, server.URL, ModeDefault); content != want {
		t.Fatalf("got content %q, want %q", content, want)
	}

	if content := getAndReadAll(t, store, server.URL, ModeCacheOnly); content != want {
		t.Errorf("slow download not cached: got %q, want %q", content, want)
	}
	assertSlotsFree(t, store)
}

func TestStalledBodyKilledByWatchdog(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "||example.com^\n")
		w.(http.Flusher).Flush()
		// Keep the connection open without sending another byte.
		<-r.Context().Done()
	}))
	defer server.Close()

	dir := t.TempDir()
	store := newTestStore(t, dir)
	store.stallTimeout = 150 * time.Millisecond

	reader, _, err := store.Get(t.Context(), server.URL, ModeDefault)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_, err = io.ReadAll(reader)
	reader.Close()
	if !errors.Is(err, errStalled) {
		t.Fatalf("got read error %v, want errStalled", err)
	}
	// The transport's own cause must stay matchable through the wrap.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("stall error hides the cancellation cause: %v", err)
	}

	assertNotCached(t, store, server.URL)
	assertNoTempFiles(t, dir)
	assertSlotsFree(t, store)
}

func TestWatchdogNotArmedDuringHeaderWait(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Exceed the stall timeout before headers are out; only the body is
		// policed by the watchdog, so this must still succeed.
		time.Sleep(400 * time.Millisecond)
		io.WriteString(w, testListContent)
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())
	store.stallTimeout = 100 * time.Millisecond

	if content := getAndReadAll(t, store, server.URL, ModeDefault); content != testListContent {
		t.Fatalf("got content %q, want %q", content, testListContent)
	}
}

func TestTransientErrorsRetried(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, testListContent)
	}))
	defer server.Close()

	store := newFastRetryStore(t, t.TempDir())

	if content := getAndReadAll(t, store, server.URL, ModeDefault); content != testListContent {
		t.Fatalf("got content %q, want %q", content, testListContent)
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("got %d requests, want 3", got)
	}
	assertSlotsFree(t, store)
}

func TestRetriesExhausted(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newFastRetryStore(t, t.TempDir())

	if _, _, err := store.Get(t.Context(), server.URL, ModeDefault); err == nil {
		t.Fatal("Get succeeded against an always-500 server")
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("got %d requests, want 3", got)
	}
	assertSlotsFree(t, store)
}

func TestFailedFetchServesStaleWithoutRetry(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
	}{
		{"TransientFailure", http.StatusInternalServerError},
		{"PermanentFailure", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			dir := t.TempDir()
			store := newFastRetryStore(t, dir)
			seedStaleEntry(t, store, server.URL)

			reader, src, err := store.Get(t.Context(), server.URL, ModeDefault)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			defer reader.Close()
			// The fallback must have been read into memory: deleting the
			// content file mid-parse must not affect the stream.
			removeCacheContent(t, dir)
			content, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if string(content) != staleListContent {
				t.Errorf("got content %q, want stale copy %q", content, staleListContent)
			}
			if src != SourceStaleCache {
				t.Errorf("got source %v, want SourceStaleCache", src)
			}
			// The fallback makes retrying pointless: a single attempt caps how
			// long a dead network stalls startup.
			if got := requests.Load(); got != 1 {
				t.Errorf("got %d requests, want 1 (no retries with a stale copy)", got)
			}
			assertSlotsFree(t, store)
		})
	}
}

func Test4xxNotRetried(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	store := newFastRetryStore(t, t.TempDir())

	if _, _, err := store.Get(t.Context(), server.URL, ModeDefault); err == nil {
		t.Fatal("Get succeeded against a 404 server")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests, want 1 (4xx is permanent)", got)
	}
	assertSlotsFree(t, store)
}

func TestSemaphoreReleasedOnAllPaths(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, testListContent)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, strings.Repeat("||example.com^\n", 1<<16))
	})
	mux.HandleFunc("/midbody", func(w http.ResponseWriter, _ *http.Request) {
		// Declaring more than gets written makes the server close the
		// connection short, so the client sees a mid-body error.
		w.Header().Set("Content-Length", "4096")
		io.WriteString(w, "||example.com^\n")
	})
	mux.HandleFunc("/notfound", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html></html>")
	})
	mux.HandleFunc("/empty", func(_ http.ResponseWriter, _ *http.Request) {})
	mux.HandleFunc("/stall", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "||example.com^\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	store := newFastRetryStore(t, t.TempDir())
	store.stallTimeout = 150 * time.Millisecond

	getAndReadAll(t, store, server.URL+"/ok", ModeDefault)

	reader, _, err := store.Get(t.Context(), server.URL+"/big", ModeDefault)
	if err != nil {
		t.Fatalf("Get /big: %v", err)
	}
	reader.Read(make([]byte, 10))
	reader.Close()

	for _, path := range []string{"/midbody", "/stall", "/empty"} {
		reader, _, err := store.Get(t.Context(), server.URL+path, ModeDefault)
		if err != nil {
			t.Fatalf("Get %s: %v", path, err)
		}
		if _, err := io.ReadAll(reader); err == nil {
			t.Fatalf("draining %s succeeded, want an error", path)
		}
		reader.Close()
	}

	for _, path := range []string{"/notfound", "/html"} {
		if _, _, err := store.Get(t.Context(), server.URL+path, ModeDefault); err == nil {
			t.Fatalf("Get %s succeeded, want an error", path)
		}
	}

	assertSlotsFree(t, store)
}

// TestNestedIncludeAtCapacityOne exercises the invariant filter.parseURL
// relies on: a goroutine holding a fetch slot drives its own stream to EOF
// without waiting on descendants, so a queued nested include acquires the
// slot as soon as the parent finishes - even at capacity 1.
func TestNestedIncludeAtCapacityOne(t *testing.T) {
	t.Parallel()

	const childContent = "||child.example.com^\n"
	mux := http.NewServeMux()
	mux.HandleFunc("/parent.txt", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, strings.Repeat("||parent.example.com^\n", 1<<10))
	})
	mux.HandleFunc("/child.txt", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, childContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	store := newTestStore(t, t.TempDir())
	store.sem = make(chan struct{}, 1)

	parent, _, err := store.Get(t.Context(), server.URL+"/parent.txt", ModeDefault)
	if err != nil {
		t.Fatalf("Get parent: %v", err)
	}
	if _, err := parent.Read(make([]byte, 10)); err != nil {
		t.Fatalf("read parent: %v", err)
	}

	// Mimic parseURL encountering an !#include mid-scan: the child fetch
	// starts on its own goroutine and queues for the sole slot.
	type outcome struct {
		content string
		err     error
	}
	childDone := make(chan outcome, 1)
	go func() {
		reader, _, err := store.Get(t.Context(), server.URL+"/child.txt", ModeDefault)
		if err != nil {
			childDone <- outcome{err: err}
			return
		}
		defer reader.Close()
		content, err := io.ReadAll(reader)
		childDone <- outcome{content: string(content), err: err}
	}()
	time.Sleep(50 * time.Millisecond)

	// The parent keeps scanning to EOF, which frees the slot.
	if _, err := io.ReadAll(parent); err != nil {
		t.Fatalf("drain parent: %v", err)
	}
	parent.Close()

	select {
	case got := <-childDone:
		if got.err != nil {
			t.Fatalf("child fetch: %v", got.err)
		}
		if got.content != childContent {
			t.Fatalf("got child content %q, want %q", got.content, childContent)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child fetch deadlocked on the fetch slot")
	}
}

func TestSingleFlightCollapsesConcurrentFetches(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		io.WriteString(w, testListContent)
	}))
	defer server.Close()

	store := newTestStore(t, t.TempDir())

	leader, _, err := store.Get(t.Context(), server.URL, ModeDefault)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// The joiner arrives while the leader's download is unfinished; it must
	// wait for the promoted copy instead of dialling again.
	type outcome struct {
		content string
		err     error
	}
	joinerDone := make(chan outcome, 1)
	go func() {
		reader, _, err := store.Get(t.Context(), server.URL, ModeDefault)
		if err != nil {
			joinerDone <- outcome{err: err}
			return
		}
		defer reader.Close()
		content, err := io.ReadAll(reader)
		joinerDone <- outcome{content: string(content), err: err}
	}()
	time.Sleep(50 * time.Millisecond)

	content, err := io.ReadAll(leader)
	leader.Close()
	if err != nil {
		t.Fatalf("drain leader: %v", err)
	}
	if string(content) != testListContent {
		t.Fatalf("got leader content %q, want %q", content, testListContent)
	}

	select {
	case got := <-joinerDone:
		if got.err != nil {
			t.Fatalf("joiner: %v", got.err)
		}
		if got.content != testListContent {
			t.Fatalf("got joiner content %q, want %q", got.content, testListContent)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("joiner never completed")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests, want 1", got)
	}
}

func TestSingleFlightLeaderFailureFallsBack(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			// Park the leader's first attempt until the joiner is waiting on
			// the flight.
			<-release
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newFastRetryStore(t, t.TempDir())

	leaderErr := make(chan error, 1)
	go func() {
		_, _, err := store.Get(t.Context(), server.URL, ModeDefault)
		leaderErr <- err
	}()
	waitFor(t, func() bool { return requests.Load() == 1 })

	joinerErr := make(chan error, 1)
	go func() {
		_, _, err := store.Get(t.Context(), server.URL, ModeDefault)
		joinerErr <- err
	}()
	// The leader is parked inside its first attempt, so its flight cannot
	// have completed yet; by the end of this sleep the joiner is waiting on
	// it, not running its own fetch.
	time.Sleep(100 * time.Millisecond)
	close(release)

	for name, ch := range map[string]chan error{"leader": leaderErr, "joiner": joinerErr} {
		select {
		case err := <-ch:
			if err == nil {
				t.Errorf("%s Get succeeded against an always-500 server", name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s Get never returned", name)
		}
	}

	// The failed leader's exit elects the joiner to lead the next flight:
	// 3 attempts each, one flight at a time, never in parallel.
	if got := requests.Load(); got != 6 {
		t.Errorf("got %d requests, want 6", got)
	}
	assertSlotsFree(t, store)
}

func TestPermanentDoFailuresNotRetried(t *testing.T) {
	t.Parallel()

	t.Run("UnsupportedScheme", func(t *testing.T) {
		t.Parallel()

		store := newFastRetryStore(t, t.TempDir())

		_, _, err := store.Get(t.Context(), "htp://example.com/list.txt", ModeDefault)
		if err == nil || !strings.Contains(err.Error(), "unsupported URL scheme") {
			t.Fatalf("got %v, want an unsupported-scheme error", err)
		}
	})

	t.Run("RedirectLoop", func(t *testing.T) {
		t.Parallel()

		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			http.Redirect(w, r, "/loop", http.StatusFound)
		}))
		defer server.Close()

		store := newFastRetryStore(t, t.TempDir())

		_, _, err := store.Get(t.Context(), server.URL, ModeDefault)
		if !errors.Is(err, errTooManyRedirects) {
			t.Fatalf("got %v, want errTooManyRedirects", err)
		}
		// One walk through the loop (the initial request plus nine followed
		// redirects), not three: a redirect loop is permanent.
		if got := requests.Load(); got != 10 {
			t.Errorf("got %d requests, want 10", got)
		}
	})
}

func TestCancelledWhileQueuedForSlot(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/hold", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "||example.com^\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	mux.HandleFunc("/queued", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, testListContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	store := newTestStore(t, t.TempDir())
	store.sem = make(chan struct{}, 1)

	holder, _, err := store.Get(t.Context(), server.URL+"/hold", ModeDefault)
	if err != nil {
		t.Fatalf("Get /hold: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := store.Get(ctx, server.URL+"/queued", ModeDefault); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}

	// The cancelled Get must leave no flight behind, or later Gets of the
	// same URL would block on it forever. (/hold's own flight is still open
	// by design - its reader is.)
	store.flightMu.Lock()
	_, pending := store.inflight[server.URL+"/queued"]
	store.flightMu.Unlock()
	if pending {
		t.Error("cancelled Get left its flight behind")
	}

	holder.Close()
	assertSlotsFree(t, store)
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
	content, _ := getAndReadAllSource(t, store, url, mode)
	return content
}

// getAndReadAllSource is getAndReadAll for tests that also assert the Source.
func getAndReadAllSource(t *testing.T, store *FilterListStore, url string, mode FetchMode) (string, Source) {
	t.Helper()
	reader, src, err := store.Get(t.Context(), url, mode)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read filter list: %v", err)
	}
	return string(content), src
}

// seedStaleEntry installs an already-expired cache entry holding
// staleListContent for url through the cache's own API, so the tests carry no
// knowledge of its on-disk format.
func seedStaleEntry(t *testing.T, store *FilterListStore, url string) {
	t.Helper()
	tempFile, err := store.cache.TempFile()
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	if _, err := tempFile.WriteString(staleListContent); err != nil {
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

// removeCacheContent deletes every cache content file, simulating an OS cache
// purge that spares the index. The one place tests touch the on-disk naming.
func removeCacheContent(t *testing.T, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.cache.txt"))
	if err != nil {
		t.Fatalf("glob cache files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no cache content files to remove")
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			t.Fatalf("remove %s: %v", f, err)
		}
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

// newFastRetryStore returns a store whose retry backoff is near-instant, for
// tests that exercise the retry policy.
func newFastRetryStore(t *testing.T, dir string) *FilterListStore {
	t.Helper()
	store := newTestStore(t, dir)
	store.retryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	return store
}

// waitFor polls cond until it holds or a deadline lapses.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// assertSlotsFree verifies every fetch slot has been released: slot leaks
// starve later fetches forever.
func assertSlotsFree(t *testing.T, store *FilterListStore) {
	t.Helper()
	for i := 0; i < cap(store.sem); i++ {
		select {
		case store.sem <- struct{}{}:
		default:
			t.Fatalf("only %d of %d fetch slots free", i, cap(store.sem))
		}
	}
	for range cap(store.sem) {
		<-store.sem
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

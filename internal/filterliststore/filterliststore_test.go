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

	reader, err := store.Get(t.Context(), server.URL, ModeDefault)
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

	reader, err := store.Get(t.Context(), server.URL, ModeDefault)
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

	reader, err := store.Get(t.Context(), server.URL, ModeDefault)
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

			if _, err := store.Get(t.Context(), server.URL, ModeDefault); err == nil {
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

	if _, err := store.Get(t.Context(), server.URL, ModeCacheOnly); err == nil {
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

	reader, err := store.Get(t.Context(), server.URL, ModeDefault)
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

	if _, err := store.Get(t.Context(), server.URL, ModeDefault); err == nil {
		t.Fatal("Get succeeded against an always-500 server")
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("got %d requests, want 3", got)
	}
	assertSlotsFree(t, store)
}

func TestNoRetryWithStaleCopy(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newFastRetryStore(t, t.TempDir())
	seedStaleEntry(t, store, server.URL, "||stale.example.com^\n")

	// A stale copy caps the damage of a failed fetch, so a single attempt is
	// enough; the fallback itself arrives in a later stage.
	if _, err := store.Get(t.Context(), server.URL, ModeDefault); err == nil {
		t.Fatal("Get succeeded against an always-500 server")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests, want 1 (no retries with a stale copy)", got)
	}
	assertSlotsFree(t, store)
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

	if _, err := store.Get(t.Context(), server.URL, ModeDefault); err == nil {
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

	reader, err := store.Get(t.Context(), server.URL+"/big", ModeDefault)
	if err != nil {
		t.Fatalf("Get /big: %v", err)
	}
	reader.Read(make([]byte, 10))
	reader.Close()

	for _, path := range []string{"/midbody", "/stall", "/empty"} {
		reader, err := store.Get(t.Context(), server.URL+path, ModeDefault)
		if err != nil {
			t.Fatalf("Get %s: %v", path, err)
		}
		if _, err := io.ReadAll(reader); err == nil {
			t.Fatalf("draining %s succeeded, want an error", path)
		}
		reader.Close()
	}

	for _, path := range []string{"/notfound", "/html"} {
		if _, err := store.Get(t.Context(), server.URL+path, ModeDefault); err == nil {
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

	parent, err := store.Get(t.Context(), server.URL+"/parent.txt", ModeDefault)
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
		reader, err := store.Get(t.Context(), server.URL+"/child.txt", ModeDefault)
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

	leader, err := store.Get(t.Context(), server.URL, ModeDefault)
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
		reader, err := store.Get(t.Context(), server.URL, ModeDefault)
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
		_, err := store.Get(t.Context(), server.URL, ModeDefault)
		leaderErr <- err
	}()
	waitFor(t, func() bool { return requests.Load() == 1 })

	joinerErr := make(chan error, 1)
	go func() {
		_, err := store.Get(t.Context(), server.URL, ModeDefault)
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

		_, err := store.Get(t.Context(), "htp://example.com/list.txt", ModeDefault)
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

		_, err := store.Get(t.Context(), server.URL, ModeDefault)
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

	holder, err := store.Get(t.Context(), server.URL+"/hold", ModeDefault)
	if err != nil {
		t.Fatalf("Get /hold: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Get(ctx, server.URL+"/queued", ModeDefault); !errors.Is(err, context.Canceled) {
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
	reader, err := store.Get(t.Context(), url, mode)
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

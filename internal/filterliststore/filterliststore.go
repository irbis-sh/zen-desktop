package filterliststore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"mime"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/filterliststore/diskcache"
)

const (
	defaultExpiry = 24 * time.Hour

	// fetchConcurrency caps concurrent list downloads. Benchmarked across
	// normal, slow, high-latency and lossy links (#766): on normal connections
	// gains flatten past 4, while 1-2 serialise stall-watchdog waits. Since a
	// slot is held while the caller consumes the stream, the cap also bounds
	// parse parallelism for network-served lists (cache hits bypass it), so it
	// should stay within the core count of the smallest machines Zen targets.
	fetchConcurrency = 4

	// defaultStallTimeout is how long a response body may go without yielding
	// a single byte before the download is treated as dead.
	defaultStallTimeout = 30 * time.Second
)

// errTooManyRedirects marks a redirect loop - a server misconfiguration that
// retrying cannot fix.
var errTooManyRedirects = errors.New("stopped after 10 redirects")

var (
	httpClient = &http.Client{
		// No overall timeout: it would cover the entire body, and large lists
		// on slow links are exactly the case being fixed (#766). Each
		// connection phase has its own budget in the transport, and body
		// progress is policed by the stall watchdog.
		Transport: newTransport(),
		// Same 10-redirect cap as the default policy, but with a sentinel the
		// retry classifier can match: a redirect loop is permanent, unlike the
		// network conditions other Do errors report.
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errTooManyRedirects
			}
			return nil
		},
	}
	// headerRegex matches comments prefixed with a hash and [Adblock Plus 2.0]-style headers.
	headerRegex = regexp.MustCompile(`^(?:!|\[|#[^#%@$])`)
)

// newTransport clones http.DefaultTransport rather than building a fresh
// *http.Transport: a zero-value transport would silently drop
// ProxyFromEnvironment (breaking corporate-proxy setups) and HTTP/2 support
// (de-multiplexing fetches that share a CDN connection).
func newTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	// Covers the wait for response headers on HTTP/1 and, through the bundled
	// http2 transport, HTTP/2 as well.
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ForceAttemptHTTP2 = true
	return transport
}

// FetchMode controls how Get balances the cache against the network.
type FetchMode int

const (
	// ModeDefault serves fresh cache entries and fetches otherwise.
	ModeDefault FetchMode = iota
	// ModePreferCache serves any cache entry, stale ones included, and only
	// fetches on a cache miss.
	ModePreferCache
	// ModeCacheOnly never touches the network; a cache miss is an error.
	ModeCacheOnly
)

type FilterListStore struct {
	cache *diskcache.Cache

	// sem holds the fetch slots: acquired before the dial, released at body
	// EOF or Close, whichever comes first. Cache hits bypass it.
	sem chan struct{}

	flightMu sync.Mutex
	inflight map[string]flight

	// Overridable in tests.
	client       *http.Client
	stallTimeout time.Duration
	retryDelays  []time.Duration
}

// flight tracks an in-progress download so concurrent Gets of the same URL
// collapse into one; it is closed when the download reaches its terminal
// point. The include deduplication in filter.AddURL is per-call: two lists
// sharing an !#include would otherwise download it twice, burning a scarce
// fetch slot and racing promotion. When a flight's leader fails, the waiters
// race to lead a new flight, so a sick URL is probed by one fetch at a time
// instead of by every waiter at once.
type flight chan struct{}

func New(cachePath string) (*FilterListStore, error) {
	cache, err := diskcache.New(cachePath)
	if err != nil {
		return nil, fmt.Errorf("create cache: %v", err)
	}

	return &FilterListStore{
		cache:        cache,
		sem:          make(chan struct{}, fetchConcurrency),
		inflight:     make(map[string]flight),
		client:       httpClient,
		stallTimeout: defaultStallTimeout,
		retryDelays:  []time.Duration{time.Second, 3 * time.Second},
	}, nil
}

// Get returns a stream of the filter list at url. Network-served content is
// cached as it is read: once the returned reader hits a verified EOF, the
// downloaded copy becomes the authoritative cache entry. A download that breaks
// mid-body surfaces the failure as an error from Read, so a consumer draining
// the stream (e.g. via bufio.Scanner) always learns it saw truncated content.
//
// ctx cancels every phase of a fetch: the wait for a fetch slot, the request
// itself, and the body stream. mode controls how the cache is balanced against
// the network.
//
// A network-served reader must be read to EOF or an error, or closed: until
// one of those happens it holds one of the store's fetch slots, and concurrent
// Gets of the same URL wait for its outcome. Abandoning a reader starves both.
func (st *FilterListStore) Get(ctx context.Context, url string, mode FetchMode) (io.ReadCloser, error) {
	for {
		content, hasStale := st.loadCache(url, mode)
		if content != nil {
			log.Printf("loading %q from cache", url)
			return content, nil
		}

		if mode == ModeCacheOnly {
			return nil, fmt.Errorf("no cached copy of %q", url)
		}

		f, leader := st.enterFlight(url)
		if leader {
			return st.fetch(ctx, url, hasStale, func() { st.exitFlight(url) })
		}
		// Another Get is already downloading url: wait for it, then loop to
		// serve the copy it promoted. If the leader failed instead, the next
		// round's enterFlight elects a new leader among the waiters. The loop
		// terminates because every round retires at least one goroutine - the
		// round's leader returns its result directly.
		select {
		case <-f:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// loadCache returns a reader when the cache can satisfy mode, or nil when a
// fetch is needed. hasStale reports whether a stale entry remains on disk as a
// fallback for a failed fetch.
func (st *FilterListStore) loadCache(url string, mode FetchMode) (content io.ReadCloser, hasStale bool) {
	content, meta, err := st.cache.Load(url)
	if err != nil {
		log.Printf("failed to load from cache: %v", err)
		return nil, false
	}
	if content == nil {
		return nil, false
	}
	if mode != ModeDefault || meta.IsFresh() {
		return content, false
	}
	// Stale entries are kept on disk as a fallback for failed fetches,
	// but a fetch is still attempted first.
	content.Close()
	return nil, true
}

func (st *FilterListStore) enterFlight(url string) (f flight, leader bool) {
	st.flightMu.Lock()
	defer st.flightMu.Unlock()
	if f, ok := st.inflight[url]; ok {
		return f, false
	}
	f = make(flight)
	st.inflight[url] = f
	return f, true
}

func (st *FilterListStore) exitFlight(url string) {
	st.flightMu.Lock()
	f := st.inflight[url]
	delete(st.inflight, url)
	st.flightMu.Unlock()
	close(f)
}

// fetch downloads url, holding a fetch slot from before the dial until the
// returned reader hits EOF or is closed - except during backoff sleeps
// between attempts, when the slot is handed back so a failing URL does not
// delay healthy fetches queued behind it. onDone runs exactly once, when the
// fetch reaches its terminal point (or fails before producing a reader).
func (st *FilterListStore) fetch(ctx context.Context, url string, hasStale bool, onDone func()) (io.ReadCloser, error) {
	select {
	case st.sem <- struct{}{}:
	case <-ctx.Done():
		onDone()
		return nil, ctx.Err()
	}
	var once sync.Once
	finish := func() {
		once.Do(func() {
			<-st.sem
			onDone()
		})
	}

	// Retries exist to salvage a list that has nothing to fall back to. With
	// a stale copy on disk, failing fast beats retry-heroics: it caps how
	// long startup stalls on a dead network. Only pre-body failures are ever
	// retried; once body bytes have reached the caller, the sole outcome of a
	// broken stream is a truncation error from the reader.
	attempts := 3
	if hasStale {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; ; attempt++ {
		reader, transient, err := st.fetchOnce(ctx, url, finish)
		if err == nil {
			return reader, nil
		}
		lastErr = err
		if !transient || attempt >= attempts {
			break
		}
		delay := withJitter(st.retryDelays[min(attempt-1, len(st.retryDelays)-1)])
		log.Printf("fetching %q failed (attempt %d of %d), retrying in %v: %v", url, attempt, attempts, delay, err)
		// The slot is not held while sleeping. On these two early returns the
		// slot is already released, so finish must not run - onDone directly.
		<-st.sem
		if !sleepCtx(ctx, delay) {
			onDone()
			return nil, ctx.Err()
		}
		select {
		case st.sem <- struct{}{}:
		case <-ctx.Done():
			onDone()
			return nil, ctx.Err()
		}
	}
	finish()
	return nil, lastErr
}

// fetchOnce makes a single fetch attempt. transient reports whether the
// failure is worth retrying: network-level errors and 5xx responses are,
// 4xx responses and non-list content are not. On success the returned reader
// owns the request lifecycle and calls finish at its terminal point.
func (st *FilterListStore) fetchOnce(ctx context.Context, url string, finish func()) (_ io.ReadCloser, transient bool, _ error) {
	reqCtx, cancel := context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, false, fmt.Errorf("create request: %v", err)
	}
	// Do would reject this too, but its error is indistinguishable from a
	// network condition and would burn retries on a permanent typo.
	if scheme := req.URL.Scheme; scheme != "http" && scheme != "https" {
		cancel()
		return nil, false, fmt.Errorf("unsupported URL scheme %q", scheme)
	}

	resp, err := st.client.Do(req) // #nosec G704 -- URL is from configured filter lists, not arbitrary user input
	if err != nil {
		cancel()
		// With redirect loops carved out, errors out of Do are network
		// conditions (dial, TLS, header timeout, resets) - worth retrying
		// unless the caller's own context ended.
		transient := ctx.Err() == nil && !errors.Is(err, errTooManyRedirects)
		return nil, transient, fmt.Errorf("do request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, resp.StatusCode >= http.StatusInternalServerError, fmt.Errorf("non-200 response: %q", resp.Status)
	}

	// A 200 carrying HTML is a captive portal or a misconfigured server, not a
	// filter list. Under keep-forever cache semantics, promoting it would
	// install the portal page as the authoritative copy, so treat it as a
	// fetch failure instead - a permanent one, since a portal will not go away
	// within a retry backoff. The parse error is deliberately ignored:
	// ParseMediaType still returns the media type when only a parameter is
	// malformed (mime.ErrInvalidMediaParameter), and hand-rolled portal
	// servers are exactly where malformed headers come from.
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt == "text/html" {
		resp.Body.Close()
		cancel()
		return nil, false, fmt.Errorf("response content type is %q, expected a filter list", mt)
	}

	tempFile, err := st.cache.TempFile()
	if err != nil {
		// Caching is best-effort: the caller still gets the stream.
		log.Printf("failed to create temp file: %v", err)
	}

	reader := &fetchReader{
		inner: &cachingReader{
			body:          resp.Body,
			tempFile:      tempFile,
			contentLength: resp.ContentLength,
			onComplete: func(download *os.File) {
				if err := st.cacheDownload(url, download); err != nil {
					log.Printf("failed to cache %q: %v", url, err)
				}
			},
		},
		stallTimeout: st.stallTimeout,
		cancel:       cancel,
		finish:       finish,
	}
	// Headers have arrived: arm the stall watchdog over the body. Arming any
	// earlier would police the slot-queue/dial/TLS/header phases, which have
	// their own budgets, and would kill healthy slow requests. If the machine
	// sleeps mid-download, the watchdog can fire on wake even though the peer
	// is healthy; the cost is bounded - the stream reads as truncated and the
	// caller's fallback path recovers.
	reader.watchdog = time.AfterFunc(st.stallTimeout, func() {
		reader.stalled.Store(true)
		cancel()
	})
	return reader, false, nil
}

// withJitter spreads out retries of fetches that failed together, returning a
// random duration in [d/2, 3d/2).
func withJitter(d time.Duration) time.Duration {
	return d/2 + rand.N(d) // #nosec G404 -- jitter needs spread, not unpredictability
}

// sleepCtx sleeps for d; it reports false when ctx ended first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// cacheDownload installs a fully downloaded temp file as the authoritative
// cache content for url. The list's header is scanned for an "! Expires"
// directive to determine the entry's TTL. On failure the file is removed.
func (st *FilterListStore) cacheDownload(url string, download *os.File) error {
	if _, err := download.Seek(0, io.SeekStart); err != nil {
		download.Close()
		os.Remove(download.Name())
		return fmt.Errorf("rewind temp file: %v", err)
	}
	ttl := parseCacheTTL(download)
	if ttl == 0 {
		ttl = defaultExpiry
	}
	// The rename in Promote is atomic for the name but not the data: without a
	// sync, a crash shortly after promotion could persist the rename and the
	// index while leaving the content truncated or empty.
	if err := download.Sync(); err != nil {
		download.Close()
		os.Remove(download.Name())
		return fmt.Errorf("sync temp file: %v", err)
	}
	if err := download.Close(); err != nil {
		os.Remove(download.Name())
		return fmt.Errorf("close temp file: %v", err)
	}

	return st.cache.Promote(url, download.Name(), diskcache.Meta{
		ExpiresAt: time.Now().Add(ttl),
		TTL:       ttl,
	})
}

// parseCacheTTL extracts a TTL from a filter list's "! Expires" header comment.
// It returns 0 when the list does not declare one.
func parseCacheTTL(r io.Reader) time.Duration {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()

		if len(line) != 0 && !headerRegex.Match(line) {
			// The header block is over.
			break
		}

		ttl, err := parseExpires(line)
		switch {
		case errors.Is(err, errNotExpires):
			continue
		case err != nil:
			log.Printf("failed to parse cache TTL from %q, assuming default: %v", line, err)
			return 0
		default:
			return ttl
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("failed to scan filter list header for TTL, assuming default: %v", err)
	}
	return 0
}

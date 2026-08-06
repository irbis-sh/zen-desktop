package filterliststore

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/filterliststore/diskcache"
)

const defaultExpiry = 24 * time.Hour

var (
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}
	// headerRegex matches comments prefixed with a hash and [Adblock Plus 2.0]-style headers.
	headerRegex = regexp.MustCompile(`^(?:!|\[|#[^#%@$])`)
)

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
}

func New(cachePath string) (*FilterListStore, error) {
	cache, err := diskcache.New(cachePath)
	if err != nil {
		return nil, fmt.Errorf("create cache: %v", err)
	}

	return &FilterListStore{
		cache: cache,
	}, nil
}

// Get returns a stream of the filter list at url. Network-served content is
// cached as it is read: once the returned reader hits a verified EOF, the
// downloaded copy becomes the authoritative cache entry. A download that breaks
// mid-body surfaces the failure as an error from Read, so a consumer draining
// the stream (e.g. via bufio.Scanner) always learns it saw truncated content.
func (st *FilterListStore) Get(url string, mode FetchMode) (io.ReadCloser, error) {
	if content, meta, err := st.cache.Load(url); err != nil {
		log.Printf("failed to load from cache: %v", err)
	} else if content != nil {
		if mode != ModeDefault || meta.IsFresh() {
			log.Printf("loading %q from cache", url)
			return content, nil
		}
		// Stale entries are kept on disk as a fallback for failed fetches,
		// but a fetch is still attempted first.
		content.Close()
	}

	if mode == ModeCacheOnly {
		return nil, fmt.Errorf("no cached copy of %q", url)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %v", err)
	}

	resp, err := httpClient.Do(req) // #nosec G704 -- URL is from configured filter lists, not arbitrary user input
	if err != nil {
		return nil, fmt.Errorf("do request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("non-200 response: %q", resp.Status)
	}

	// A 200 carrying HTML is a captive portal or a misconfigured server, not a
	// filter list. Under keep-forever cache semantics, promoting it would
	// install the portal page as the authoritative copy, so treat it as a
	// fetch failure instead. The parse error is deliberately ignored:
	// ParseMediaType still returns the media type when only a parameter is
	// malformed (mime.ErrInvalidMediaParameter), and hand-rolled portal
	// servers are exactly where malformed headers come from.
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt == "text/html" {
		resp.Body.Close()
		return nil, fmt.Errorf("response content type is %q, expected a filter list", mt)
	}

	tempFile, err := st.cache.TempFile()
	if err != nil {
		// Caching is best-effort: the caller still gets the stream.
		log.Printf("failed to create temp file: %v", err)
	}

	return &cachingReader{
		body:          resp.Body,
		tempFile:      tempFile,
		contentLength: resp.ContentLength,
		onComplete: func(download *os.File) {
			if err := st.cacheDownload(url, download); err != nil {
				log.Printf("failed to cache %q: %v", url, err)
			}
		},
	}, nil
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

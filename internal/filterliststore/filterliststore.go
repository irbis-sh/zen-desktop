package filterliststore

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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

func (st *FilterListStore) Get(url string) (io.ReadCloser, error) {
	if content, err := st.cache.Load(url); err != nil {
		log.Printf("failed to load from cache: %v", err)
	} else if content != nil {
		log.Printf("loading %q from cache", url)
		return content, nil
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

	var teeBuffer bytes.Buffer
	var notifyCh <-chan struct{}
	var errCh <-chan struct{}
	resp.Body, notifyCh, errCh = newNotifyReadCloser(struct {
		io.Reader
		io.Closer
	}{
		Reader: io.TeeReader(resp.Body, &teeBuffer),
		Closer: resp.Body,
	})

	go func() {
		// The goal here is to make caching non-blocking. Data from the response body is cloned into teeBuffer,
		// and the cache is saved in a separate goroutine.
		// This allows the consumer of Get to start reading the response body without waiting for the entire response to be fetched.
		select {
		case <-errCh:
			// An error occurred while reading the response body, so the response should not be cached.
			return
		case <-notifyCh:
			// The response body has been closed, and we can proceed to cache the content.
		}

		cacheContent, _ := io.ReadAll(&teeBuffer) // err is always nil with bytes.Buffer.

		var cacheTTL time.Duration
		scanner := bufio.NewScanner(bytes.NewReader(cacheContent))

	outer:
		for scanner.Scan() {
			line := scanner.Bytes()

			if len(line) != 0 && !headerRegex.Match(line) {
				// Stop scanning for "! Expires" if we encounter a non-comment line.
				break
			}

			cacheTTL, err = parseExpires(line)
			switch {
			case errors.Is(err, errNotExpires):
				continue
			case err != nil:
				log.Printf("failed to parse cache TTL from %q, assuming default: %v", line, err)
				break outer
			default:
				break outer
			}
		}

		if cacheTTL == 0 {
			// Default to 24 hours if no expiry is found.
			cacheTTL = defaultExpiry
		}
		expiresAt := time.Now().Add(cacheTTL) // time.Now() might deviate from the time the request was received, but it isn't critical.

		if err := st.cache.Save(url, cacheContent, expiresAt); err != nil {
			log.Printf("failed to store in cache: %v", err)
		}
	}()

	return resp.Body, nil
}

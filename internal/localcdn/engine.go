package localcdn

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/blang/semver"
	"github.com/irbis-sh/zen-desktop/internal/config"
)

// Result describes the outcome of the local resource engine for one request.
type Result struct {
	// Served is true when a local copy of the resource was returned.
	Served bool
	// Blocked is true when the request was blocked because no local copy
	// exists and "block missing resources" is enabled.
	Blocked bool
	// Library is the key of the served library, e.g. "jquery".
	Library string
	// Version is the version of the served local copy.
	Version string
	// RequestedVersion is the version requested in the URL, when the matched
	// pattern captured a {version} placeholder.
	RequestedVersion string
	// CDNHost is the hostname of the CDN the request was intended for.
	CDNHost string
}

// VersionDelta compares a requested version with the served version and
// returns "upgrade", "downgrade", or "" when the versions are equal, missing,
// or cannot be parsed.
func VersionDelta(requested, served string) string {
	if requested == "" || served == "" {
		return ""
	}
	requestedVersion, err := semver.ParseTolerant(requested)
	if err != nil {
		return ""
	}
	servedVersion, err := semver.ParseTolerant(served)
	if err != nil {
		return ""
	}
	switch {
	case servedVersion.GT(requestedVersion):
		return "upgrade"
	case servedVersion.LT(requestedVersion):
		return "downgrade"
	default:
		return ""
	}
}

// Options configures a new Engine.
type Options struct {
	// Settings is the engine configuration snapshot.
	Settings config.LocalResources
	// IsExcluded reports whether a host is excluded from proxying. It may be
	// nil, in which case no hosts are excluded by the engine.
	IsExcluded func(host string) bool
	// IsUserExcluded reports whether a host is in the user's explicit ignored
	// hosts list. User-configured exclusions take precedence over the CDN host
	// override. It may be nil.
	IsUserExcluded func(host string) bool
	// Stats is the shared injection counter. When nil, a fresh counter is used.
	Stats *Stats
	// SaveStats persists a stats snapshot. It may be nil.
	SaveStats func(config.LocalResourcesStats) error
}

// Engine serves local copies of CDN resources and rewrites HTML responses so
// that browsers accept the locally served replacements.
//
// The engine is safe for concurrent use: matching is read-only after creation
// and all mutable state is synchronized.
type Engine struct {
	registry            *Registry
	enabled             bool
	blockMissing        bool
	allLibrariesEnabled bool
	enabledLibraries    map[string]bool
	customDir           string
	customFS            fs.FS
	stats               *Stats
	saveStats           func(config.LocalResourcesStats) error
	isExcluded          func(host string) bool
	isUserExcluded      func(host string) bool
	records             atomic.Int64
}

// statsSaveInterval is how many served resources trigger a stats persist.
const statsSaveInterval = 64

// NewEngine creates an engine from the given options and the embedded registry.
func NewEngine(opts Options) (*Engine, error) {
	registry, err := NewRegistry(resourcesManifest, opts.Settings.CustomMappings)
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}

	stats := opts.Stats
	if stats == nil {
		stats = newStats(config.LocalResourcesStats{
			ByLibrary: make(map[string]int64),
			ByCDN:     make(map[string]int64),
		})
	}

	e := &Engine{
		registry:            registry,
		enabled:             opts.Settings.Enabled,
		blockMissing:        opts.Settings.BlockMissing,
		allLibrariesEnabled: len(opts.Settings.EnabledLibraries) == 0,
		enabledLibraries:    libraryEnabledSet(opts.Settings.EnabledLibraries),
		customDir:           strings.TrimSpace(opts.Settings.CustomDir),
		stats:               stats,
		saveStats:           opts.SaveStats,
		isExcluded:          opts.IsExcluded,
		isUserExcluded:      opts.IsUserExcluded,
	}
	if e.customDir != "" {
		e.customFS = os.DirFS(e.customDir)
	}
	return e, nil
}

// HandleRequest intercepts a proxied request. When the URL matches a bundled
// local resource, the local copy is returned instead of forwarding the request
// to the remote CDN. When "block missing resources" is enabled and the request
// targets a known CDN host without a local copy, a 204 No Content response is
// returned. Otherwise the request passes through unchanged.
func (e *Engine) HandleRequest(req *http.Request) (*http.Response, Result, error) {
	host := strings.ToLower(req.URL.Hostname())
	excluded := e.isExcluded != nil && e.isExcluded(host)
	userExcluded := e.isUserExcluded != nil && e.isUserExcluded(host)
	if !e.enabled {
		return nil, Result{}, nil
	}

	if host == "" {
		return nil, Result{}, nil
	}
	// User-configured exclusions always win. Otherwise, known CDN hosts are
	// eligible for interception even when a built-in parent domain is excluded
	// (e.g. cdnjs.cloudflare.com under cloudflare.com).
	if userExcluded || (excluded && !e.registry.isKnownCDNHost(host)) {
		return nil, Result{}, nil
	}

	res, requestedVersion := e.registry.match(host, req.URL.Path, req.URL.Query())
	if res != nil && e.libraryEnabled(res.library.Key) {
		data, err := e.readFile(res)
		if err != nil {
			log.Printf("localcdn: reading resource %q: %v", res.mapping.File, err)
		} else {
			e.stats.record(res.library.Key, host)
			e.maybeSaveStats()
			return serveLocalResponse(req, res, data), Result{
				Served:           true,
				Library:          res.library.Key,
				Version:          res.mapping.Version,
				RequestedVersion: requestedVersion,
				CDNHost:          host,
			}, nil
		}
	}

	if e.blockMissing && e.registry.isKnownCDNHost(host) {
		return blockMissingResponse(req), Result{Blocked: true, CDNHost: host}, nil
	}

	return nil, Result{}, nil
}

// HandleResponse rewrites HTML responses, removing integrity and crossorigin
// attributes from tags that reference registry resources.
func (e *Engine) HandleResponse(_ *http.Request, res *http.Response) error {
	if !e.enabled {
		return nil
	}
	if !isHTMLResponse(res) {
		return nil
	}
	return FilterHTML(res, e.matchesURL)
}

// Flush persists the current stats snapshot. Call it before shutting down the
// proxy so counters are not lost.
func (e *Engine) Flush() {
	e.saveStatsNow()
}

// libraryEnabled reports whether a library is enabled by the current settings.
func (e *Engine) libraryEnabled(key string) bool {
	if e.allLibrariesEnabled {
		return true
	}
	return e.enabledLibraries[key]
}

// matchesURL reports whether an HTML tag URL references a servable resource.
func (e *Engine) matchesURL(rawURL string) bool {
	if !e.enabled {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Host == "" {
		if !strings.HasPrefix(rawURL, "//") {
			return false
		}
		u, err = url.Parse("http:" + rawURL)
		if err != nil {
			return false
		}
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	if e.isUserExcluded != nil && e.isUserExcluded(host) {
		return false
	}
	if e.isExcluded != nil && e.isExcluded(host) && !e.registry.isKnownCDNHost(host) {
		return false
	}

	res, _ := e.registry.match(host, u.Path, u.Query())
	return res != nil && e.libraryEnabled(res.library.Key)
}

// readFile reads the resource content from the embedded files or the custom
// resource directory.
func (e *Engine) readFile(res *resource) ([]byte, error) {
	filePath, err := cleanRelativePath(res.mapping.File)
	if err != nil {
		return nil, err
	}
	if res.custom {
		if e.customFS == nil {
			return nil, errors.New("custom resource directory is not configured")
		}
		return fs.ReadFile(e.customFS, filePath)
	}
	return fs.ReadFile(embeddedResources, filePath)
}

// cleanRelativePath normalizes a resource file path and rejects paths that
// escape the resource root.
func cleanRelativePath(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':') {
		return "", errors.New("resource file path must be relative")
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("resource file path escapes the resource root")
	}
	return clean, nil
}

// serveLocalResponse builds the HTTP response for a locally served resource.
func serveLocalResponse(req *http.Request, res *resource, data []byte) *http.Response {
	contentType := res.mapping.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	header := make(http.Header)
	header.Set("Content-Type", contentType)
	header.Set("Content-Length", strconv.Itoa(len(data)))
	header.Set("Cache-Control", "public, max-age=31536000, immutable")
	header.Set("Access-Control-Allow-Origin", "*")

	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        http.StatusText(http.StatusOK),
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Header:        header,
		ContentLength: int64(len(data)),
		Body:          io.NopCloser(bytes.NewReader(data)),
		Request:       req,
	}
}

// blockMissingResponse builds the response for a blocked missing resource.
func blockMissingResponse(req *http.Request) *http.Response {
	header := make(http.Header)
	header.Set("Cache-Control", "no-store")

	return &http.Response{
		StatusCode: http.StatusNoContent,
		Status:     http.StatusText(http.StatusNoContent),
		Proto:      req.Proto,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Header:     header,
		Request:    req,
	}
}

// isHTMLResponse reports whether the response is an HTML document.
func isHTMLResponse(res *http.Response) bool {
	mediaType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mediaType == "text/html"
}

// maybeSaveStats persists stats every statsSaveInterval records.
func (e *Engine) maybeSaveStats() {
	if e.saveStats == nil {
		return
	}
	if e.records.Add(1)%statsSaveInterval != 0 {
		return
	}
	e.saveStatsNow()
}

// saveStatsNow persists the current stats snapshot.
func (e *Engine) saveStatsNow() {
	if e.saveStats == nil {
		return
	}
	if err := e.saveStats(e.stats.Snapshot()); err != nil {
		log.Printf("localcdn: saving stats: %v", err)
	}
}

// libraryEnabledSet converts an enabled-library list into a lookup map.
func libraryEnabledSet(enabledLibraries []string) map[string]bool {
	enabled := make(map[string]bool, len(enabledLibraries))
	for _, key := range enabledLibraries {
		enabled[key] = true
	}
	return enabled
}

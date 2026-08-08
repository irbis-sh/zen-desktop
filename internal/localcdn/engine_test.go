package localcdn

import (
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/config"
)

func engineTestRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return &http.Request{
		Method:     http.MethodGet,
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
}

func newEngineForTest(t *testing.T, opts Options) *Engine {
	t.Helper()
	engine, err := NewEngine(opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine
}

func TestEngineServesLocalResource(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
	})

	resp, result, err := engine.HandleRequest(engineTestRequest(t, "https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp == nil {
		t.Fatal("expected local response, got nil")
	}
	if !result.Served {
		t.Fatalf("expected served result, got %+v", result)
	}
	if result.Library != "jquery" {
		t.Fatalf("library = %q, want jquery", result.Library)
	}
	if result.Version != "3.7.1" {
		t.Fatalf("version = %q, want 3.7.1", result.Version)
	}
	if result.CDNHost != "ajax.googleapis.com" {
		t.Fatalf("cdn host = %q, want ajax.googleapis.com", result.CDNHost)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Fatalf("content type = %q, want application/javascript", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", cc)
	}
	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "*" {
		t.Fatalf("access-control-allow-origin = %q, want *", acao)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.ContentLength != int64(len(body)) {
		t.Fatalf("content length = %d, body length = %d", resp.ContentLength, len(body))
	}
	if !strings.Contains(string(body), "jQuery v3.7.1") {
		t.Fatal("body does not look like jQuery 3.7.1")
	}

	embedded, err := fs.ReadFile(embeddedResources, "resources/jquery/3.7.1/jquery.min.js")
	if err != nil {
		t.Fatalf("read embedded file: %v", err)
	}
	if string(body) != string(embedded) {
		t.Fatal("served body differs from embedded file")
	}
}

func TestEngineServesFromMultipleCDNs(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
	})

	urls := []string{
		"https://code.jquery.com/jquery-3.7.1.min.js",
		"https://cdnjs.cloudflare.com/ajax/libs/jquery/3.7.1/jquery.min.js",
		"https://cdn.jsdelivr.net/npm/jquery@3.7.1/dist/jquery.min.js",
		"https://unpkg.com/jquery@3.7.1/dist/jquery.min.js",
	}
	for _, rawURL := range urls {
		resp, result, err := engine.HandleRequest(engineTestRequest(t, rawURL))
		if err != nil {
			t.Fatalf("HandleRequest(%s): %v", rawURL, err)
		}
		if resp == nil || !result.Served {
			t.Fatalf("HandleRequest(%s) not served", rawURL)
		}
		resp.Body.Close()
	}
}

// TestEngineServesBitchuteReference verifies that the bundled registry serves
// the resources LocalCDN intercepts on old.bitchute.com (see the round-2 bug
// report). Requested versions may differ from the bundled ones; version
// ranges must cover them.
func TestEngineServesBitchuteReference(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
	})

	urls := map[string]string{
		"https://cdnjs.cloudflare.com/ajax/libs/jquery/3.7.1/jquery.min.js":                           "jquery",
		"https://cdnjs.cloudflare.com/ajax/libs/js-cookie/2.2.1/js.cookie.min.js":                     "jscookie",
		"https://cdnjs.cloudflare.com/ajax/libs/moment.js/2.29.4/moment.min.js":                       "moment",
		"https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/3.4.1/css/bootstrap.min.css":        "bootstrap",
		"https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/3.4.1/js/bootstrap.min.js":          "bootstrap",
		"https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/11.0.2/bootstrap-slider.min.js":      "bootstrapslider",
		"https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/11.0.2/css/bootstrap-slider.min.css": "bootstrapslider",
		"https://cdnjs.cloudflare.com/ajax/libs/animate.css/4.1.1/animate.min.css":                    "animatecss",
		"https://cdnjs.cloudflare.com/ajax/libs/toastr.js/2.1.4/toastr.min.js":                        "toastr",
		"https://cdnjs.cloudflare.com/ajax/libs/toastr.js/2.1.4/toastr.min.css":                       "toastr",
		"https://cdnjs.cloudflare.com/ajax/libs/plyr/3.7.8/plyr.min.js":                               "plyr",
		"https://cdnjs.cloudflare.com/ajax/libs/plyr/3.7.8/plyr.min.css":                              "plyr",
		"https://cdnjs.cloudflare.com/ajax/libs/rickshaw/1.7.1/rickshaw.min.js":                       "rickshaw",
		"https://cdnjs.cloudflare.com/ajax/libs/rickshaw/1.7.1/rickshaw.min.css":                      "rickshaw",
		"https://cdnjs.cloudflare.com/ajax/libs/wow/1.1.2/wow.min.js":                                 "wow",
		"https://cdnjs.cloudflare.com/ajax/libs/jeditable.js/1.20.0/jquery.jeditable.min.js":          "jeditable",
		"https://cdnjs.cloudflare.com/ajax/libs/jquery-validate/1.20.0/jquery.validate.min.js":        "jqueryvalidate",
		"https://cdnjs.cloudflare.com/ajax/libs/lazysizes/5.3.2/lazysizes.min.js":                     "lazysizes",
		"https://cdnjs.cloudflare.com/ajax/libs/clipboard.js/2.0.11/clipboard.min.js":                 "clipboard",
		"https://cdnjs.cloudflare.com/ajax/libs/d3/7.8.5/d3.min.js":                                   "d3",
		"https://cdn.jsdelivr.net/npm/p2p-media-loader-core@0.6.2/build/p2p-media-loader-core.min.js": "p2pmediacore",
	}

	for rawURL, wantLibrary := range urls {
		resp, result, err := engine.HandleRequest(engineTestRequest(t, rawURL))
		if err != nil {
			t.Fatalf("HandleRequest(%s): %v", rawURL, err)
		}
		if resp == nil || !result.Served {
			t.Fatalf("HandleRequest(%s) not served", rawURL)
		}
		if result.Library != wantLibrary {
			t.Fatalf("HandleRequest(%s) library = %q, want %q", rawURL, result.Library, wantLibrary)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("HandleRequest(%s) status = %d, want 200", rawURL, resp.StatusCode)
		}
		if resp.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Fatalf("HandleRequest(%s) missing immutable cache headers", rawURL)
		}
		resp.Body.Close()
	}
}

func TestEngineBlockMissing(t *testing.T) {
	t.Parallel()

	t.Run("enabled blocks missing resource", func(t *testing.T) {
		t.Parallel()
		engine := newEngineForTest(t, Options{
			Settings: config.LocalResources{Enabled: true, BlockMissing: true},
		})

		resp, result, err := engine.HandleRequest(engineTestRequest(t, "https://cdnjs.cloudflare.com/ajax/libs/no-such-library/1.0.0/no-such.js"))
		if err != nil {
			t.Fatalf("HandleRequest: %v", err)
		}
		if resp == nil {
			t.Fatal("expected block response, got nil")
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", resp.StatusCode)
		}
		if !result.Blocked {
			t.Fatalf("expected blocked result, got %+v", result)
		}
		if result.CDNHost != "cdnjs.cloudflare.com" {
			t.Fatalf("cdn host = %q", result.CDNHost)
		}
	})

	t.Run("disabled passes through", func(t *testing.T) {
		t.Parallel()
		engine := newEngineForTest(t, Options{
			Settings: config.LocalResources{Enabled: true, BlockMissing: false},
		})

		resp, result, err := engine.HandleRequest(engineTestRequest(t, "https://cdnjs.cloudflare.com/ajax/libs/no-such-library/1.0.0/no-such.js"))
		if err != nil {
			t.Fatalf("HandleRequest: %v", err)
		}
		if resp != nil {
			t.Fatal("expected nil response, got block response")
		}
		if result.Served || result.Blocked {
			t.Fatalf("expected empty result, got %+v", result)
		}
	})

	t.Run("block missing only applies to known CDN hosts", func(t *testing.T) {
		t.Parallel()
		engine := newEngineForTest(t, Options{
			Settings: config.LocalResources{Enabled: true, BlockMissing: true},
		})

		resp, _, err := engine.HandleRequest(engineTestRequest(t, "https://example.org/some/script.js"))
		if err != nil {
			t.Fatalf("HandleRequest: %v", err)
		}
		if resp != nil {
			t.Fatal("expected pass-through for unknown host")
		}
	})
}

func TestEngineDisabled(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: false},
	})

	resp, result, err := engine.HandleRequest(engineTestRequest(t, "https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp != nil {
		t.Fatal("expected pass-through when engine is disabled")
	}
	if result.Served || result.Blocked {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestEngineRespectsExcludedHosts(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
		IsExcluded: func(host string) bool {
			return host == "example.com"
		},
	})

	resp, _, err := engine.HandleRequest(engineTestRequest(t, "https://example.com/some/script.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp != nil {
		t.Fatal("expected pass-through for excluded host")
	}
}

// TestEngineIgnoresExclusionForKnownCDNHosts verifies that a parent-domain
// exclusion (e.g. cloudflare.com excluding cdnjs.cloudflare.com) does not stop
// the engine from intercepting hosts it has bundled resources for.
func TestEngineIgnoresExclusionForKnownCDNHosts(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
		IsExcluded: func(string) bool {
			// Simulate the broad cloudflare.com exclusion matching every host.
			return true
		},
	})

	cdnjsResp, result, err := engine.HandleRequest(engineTestRequest(t, "https://cdnjs.cloudflare.com/ajax/libs/jquery/3.7.1/jquery.min.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if cdnjsResp == nil || !result.Served {
		t.Fatal("expected known CDN host to be served despite exclusion")
	}
	cdnjsResp.Body.Close()

	unpkgResp, result, err := engine.HandleRequest(engineTestRequest(t, "https://unpkg.com/vue@3.5.22/dist/vue.global.prod.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if unpkgResp == nil || !result.Served {
		t.Fatal("expected known CDN host to be served despite exclusion")
	}
	unpkgResp.Body.Close()

	exampleResp, _, err := engine.HandleRequest(engineTestRequest(t, "https://example.com/some/script.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if exampleResp != nil {
		t.Fatal("expected unknown excluded host to pass through")
	}
}

// TestEngineUserExclusionOverridesKnownCDNHost verifies that an explicit
// user-configured ignored host wins over the CDN host override.
func TestEngineUserExclusionOverridesKnownCDNHost(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
		IsUserExcluded: func(host string) bool {
			return host == "cdnjs.cloudflare.com"
		},
	})

	resp, _, err := engine.HandleRequest(engineTestRequest(t, "https://cdnjs.cloudflare.com/ajax/libs/jquery/3.7.1/jquery.min.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp != nil {
		t.Fatal("expected user-excluded host to pass through")
	}
}

func TestEngineRespectsDisabledLibraries(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{
			Enabled:          true,
			EnabledLibraries: []string{"bootstrap"},
		},
	})

	jqueryResp, _, err := engine.HandleRequest(engineTestRequest(t, "https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if jqueryResp != nil {
		t.Fatal("expected pass-through for disabled library")
	}

	bootstrapResp, result, err := engine.HandleRequest(engineTestRequest(t, "https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/5.3.8/css/bootstrap.min.css"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if bootstrapResp == nil || !result.Served {
		t.Fatal("expected enabled library to be served")
	}
	bootstrapResp.Body.Close()
}

func TestEngineServesCustomMappings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom.js"), []byte("custom-content"), 0644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{
			Enabled:   true,
			CustomDir: dir,
			CustomMappings: []config.LocalResourceMapping{
				{
					ID:          "custom-1",
					Library:     "custom",
					Version:     "1.0.0",
					Patterns:    []string{"https://cdn.example.com/custom/{version}/custom.js"},
					File:        "custom.js",
					ContentType: "application/javascript; charset=utf-8",
				},
			},
		},
	})

	resp, result, err := engine.HandleRequest(engineTestRequest(t, "https://cdn.example.com/custom/1.0.0/custom.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp == nil || !result.Served {
		t.Fatal("expected custom resource to be served")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "custom-content" {
		t.Fatalf("body = %q, want custom-content", body)
	}
}

func TestEngineRecordsStats(t *testing.T) {
	t.Parallel()

	stats := newStats(config.LocalResourcesStats{
		TotalSinceInstall: 5,
		TotalSinceReset:   3,
		ByLibrary:         map[string]int64{"vue": 3},
		ByCDN:             map[string]int64{"unpkg.com": 3},
	})
	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
		Stats:    stats,
	})

	resp, _, err := engine.HandleRequest(engineTestRequest(t, "https://unpkg.com/vue@3.5.22/dist/vue.global.prod.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	resp.Body.Close()

	snapshot := stats.Snapshot()
	if snapshot.TotalSinceInstall != 6 {
		t.Fatalf("total since install = %d, want 6", snapshot.TotalSinceInstall)
	}
	if snapshot.TotalSinceReset != 4 {
		t.Fatalf("total since reset = %d, want 4", snapshot.TotalSinceReset)
	}
	if snapshot.ByLibrary["vue"] != 4 {
		t.Fatalf("by library vue = %d, want 4", snapshot.ByLibrary["vue"])
	}
	if snapshot.ByCDN["unpkg.com"] != 4 {
		t.Fatalf("by CDN unpkg.com = %d, want 4", snapshot.ByCDN["unpkg.com"])
	}
}

func TestVersionDelta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		served    string
		want      string
	}{
		{name: "upgrade", requested: "3.6.0", served: "3.7.1", want: "upgrade"},
		{name: "downgrade", requested: "3.7.1", served: "3.6.0", want: "downgrade"},
		{name: "equal", requested: "3.7.1", served: "3.7.1", want: ""},
		{name: "unparseable requested", requested: "latest", served: "3.7.1", want: ""},
		{name: "unparseable served", requested: "3.7.1", served: "nonsense", want: ""},
		{name: "empty requested", requested: "", served: "3.7.1", want: ""},
		{name: "empty served", requested: "3.7.1", served: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := VersionDelta(tt.requested, tt.served); got != tt.want {
				t.Fatalf("VersionDelta(%q, %q) = %q, want %q", tt.requested, tt.served, got, tt.want)
			}
		})
	}
}

// TestEngineRecordsRequestedVersion verifies the requested version is captured
// from the URL and the served version differs (version-range matching).
func TestEngineRecordsRequestedVersion(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
	})

	// Moment 2.29.4 is requested but 2.30.1 is bundled.
	resp, result, err := engine.HandleRequest(engineTestRequest(t, "https://cdnjs.cloudflare.com/ajax/libs/moment.js/2.29.4/moment.min.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	resp.Body.Close()
	if result.RequestedVersion != "2.29.4" {
		t.Fatalf("requested version = %q, want 2.29.4", result.RequestedVersion)
	}
	if result.Version != "2.30.1" {
		t.Fatalf("served version = %q, want 2.30.1", result.Version)
	}
	if VersionDelta(result.RequestedVersion, result.Version) != "upgrade" {
		t.Fatal("expected upgrade delta")
	}
}

func TestEngineFlushSavesStats(t *testing.T) {
	t.Parallel()

	var saved config.LocalResourcesStats
	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
		SaveStats: func(stats config.LocalResourcesStats) error {
			saved = stats
			return nil
		},
	})

	resp, _, err := engine.HandleRequest(engineTestRequest(t, "https://cdnjs.cloudflare.com/ajax/libs/lodash.js/4.17.21/lodash.min.js"))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	resp.Body.Close()
	engine.Flush()

	if saved.TotalSinceInstall != 1 {
		t.Fatalf("saved total = %d, want 1", saved.TotalSinceInstall)
	}
}

func TestEngineHandleResponseSkipsNonHTML(t *testing.T) {
	t.Parallel()

	engine := newEngineForTest(t, Options{
		Settings: config.LocalResources{Enabled: true},
	})

	res := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(`{"integrity":"keep"}`)),
	}
	if err := engine.HandleResponse(engineTestRequest(t, "https://example.com/data.json"), res); err != nil {
		t.Fatalf("HandleResponse: %v", err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != `{"integrity":"keep"}` {
		t.Fatalf("body changed: %s", body)
	}
}

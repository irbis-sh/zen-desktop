package localcdn

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/blang/semver"
	"github.com/irbis-sh/zen-desktop/internal/config"
)

// manifest is the on-disk format of resources.json.
type manifest struct {
	Version   int               `json:"version"`
	Libraries []manifestLibrary `json:"libraries"`
	CDNHosts  []string          `json:"cdnHosts"`
}

// manifestLibrary describes one library in resources.json.
type manifestLibrary struct {
	Key              string                        `json:"key"`
	Name             string                        `json:"name"`
	License          string                        `json:"license"`
	EnabledByDefault bool                          `json:"enabledByDefault"`
	Resources        []config.LocalResourceMapping `json:"resources"`
}

// Library describes a bundled or custom library.
type Library struct {
	Key       string
	Name      string
	License   string
	Enabled   bool
	Resources []*resource
}

// LibraryInfo is a serializable description of a library for the settings UI.
type LibraryInfo struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	License       string `json:"license"`
	Version       string `json:"version"`
	Enabled       bool   `json:"enabled"`
	ResourceCount int    `json:"resourceCount"`
}

// resource is a compiled registry entry: one URL pattern mapped to a local file.
type resource struct {
	mapping      config.LocalResourceMapping
	library      *Library
	host         string
	wildcardHost bool
	segments     []segment
	query        url.Values
	versionRange semver.Range
	custom       bool
}

// segment is one path segment of a compiled URL pattern.
type segment struct {
	literal  string
	wildcard bool
	greedy   bool
	version  bool
	prefix   string
	suffix   string
}

// Registry maps CDN URLs to bundled local resources.
//
// Hosts are indexed in a hash map and each host has a small, bounded list of
// compiled patterns, so matching is O(1) per request in practice.
type Registry struct {
	libraries        map[string]*Library
	byHost           map[string][]*resource
	wildcard         []*resource
	cdnHosts         map[string]struct{}
	wildcardCDNHosts map[string]struct{}
}

// NewRegistry builds a registry from a resources.json manifest and a list of
// user-defined custom mappings.
func NewRegistry(manifestData []byte, customMappings []config.LocalResourceMapping) (*Registry, error) {
	var m manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return nil, fmt.Errorf("parse resources manifest: %w", err)
	}

	r := &Registry{
		libraries:        make(map[string]*Library),
		byHost:           make(map[string][]*resource),
		cdnHosts:         make(map[string]struct{}),
		wildcardCDNHosts: make(map[string]struct{}),
	}

	for _, ml := range m.Libraries {
		lib := &Library{
			Key:       ml.Key,
			Name:      ml.Name,
			License:   ml.License,
			Enabled:   ml.EnabledByDefault,
			Resources: make([]*resource, 0, len(ml.Resources)),
		}
		r.libraries[ml.Key] = lib
		for _, mapping := range ml.Resources {
			resources, err := compileMapping(mapping, lib, false)
			if err != nil {
				return nil, fmt.Errorf("library %q: %w", ml.Key, err)
			}
			for _, res := range resources {
				lib.Resources = append(lib.Resources, res)
				r.index(res)
			}
		}
	}

	for _, mapping := range customMappings {
		key := mapping.Library
		if key == "" {
			key = "custom"
		}
		lib, ok := r.libraries[key]
		if !ok {
			lib = &Library{Key: key, Name: key, License: "custom", Enabled: true}
			r.libraries[key] = lib
		}
		resources, err := compileMapping(mapping, lib, true)
		if err != nil {
			return nil, fmt.Errorf("custom mapping %q: %w", mapping.ID, err)
		}
		for _, res := range resources {
			lib.Resources = append(lib.Resources, res)
			r.index(res)
		}
	}

	for _, host := range m.CDNHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			continue
		}
		if strings.HasPrefix(host, "*.") {
			r.wildcardCDNHosts[strings.TrimPrefix(host, "*.")] = struct{}{}
		} else {
			r.cdnHosts[host] = struct{}{}
		}
	}

	return r, nil
}

// index adds a compiled resource to the lookup tables.
func (r *Registry) index(res *resource) {
	if res.wildcardHost {
		r.wildcard = append(r.wildcard, res)
		r.wildcardCDNHosts[res.host] = struct{}{}
		return
	}
	r.byHost[res.host] = append(r.byHost[res.host], res)
	r.cdnHosts[res.host] = struct{}{}
}

// match returns the first resource matching the given host, request path, and
// query parameters, together with the version captured from the URL's
// {version} placeholder (empty when the pattern has no placeholder).
func (r *Registry) match(host, requestPath string, query url.Values) (*resource, string) {
	host = strings.ToLower(host)
	for _, res := range r.byHost[host] {
		if version, ok := res.matchWithVersion(requestPath, query); ok {
			return res, version
		}
	}
	for _, res := range r.wildcard {
		if !strings.HasSuffix(host, "."+res.host) {
			continue
		}
		if version, ok := res.matchWithVersion(requestPath, query); ok {
			return res, version
		}
	}
	return nil, ""
}

// matchWithVersion reports whether the resource matches the request path and
// query, returning the captured {version} on success.
func (res *resource) matchWithVersion(requestPath string, query url.Values) (string, bool) {
	version, ok := res.match(requestPath)
	if !ok || !res.versionInRange(version) {
		return "", false
	}
	if !res.queryMatches(query) {
		return "", false
	}
	return version, true
}

// queryMatches reports whether the request query contains every parameter
// required by the pattern.
func (res *resource) queryMatches(query url.Values) bool {
	for key, expected := range res.query {
		actual, ok := query[key]
		if !ok {
			return false
		}
		if !equalStrings(actual, expected) {
			return false
		}
	}
	return true
}

// equalStrings compares two string slices ignoring order.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		if seen[v] == 0 {
			return false
		}
		seen[v]--
	}
	return true
}

// isKnownCDNHost reports whether the host is a CDN host tracked by the registry.
func (r *Registry) isKnownCDNHost(host string) bool {
	host = strings.ToLower(host)
	if _, ok := r.cdnHosts[host]; ok {
		return true
	}
	for suffix := range r.wildcardCDNHosts {
		if strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// CDNHosts returns the exact hosts tracked by the registry, sorted. These are
// the hosts that must always be routed through the proxy so bundled resources
// can be intercepted.
func (r *Registry) CDNHosts() []string {
	hosts := make([]string, 0, len(r.cdnHosts))
	for host := range r.cdnHosts {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

// Libraries returns the registry's libraries sorted by key.
func (r *Registry) Libraries() []*Library {
	keys := make([]string, 0, len(r.libraries))
	for key := range r.libraries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	libraries := make([]*Library, 0, len(keys))
	for _, key := range keys {
		libraries = append(libraries, r.libraries[key])
	}
	return libraries
}

// LibraryInfos returns serializable library descriptions with their current
// enabled state. An empty enabledLibraries list enables every library.
func (r *Registry) LibraryInfos(enabledLibraries []string) []LibraryInfo {
	enabledSet := make(map[string]bool, len(enabledLibraries))
	for _, key := range enabledLibraries {
		enabledSet[key] = true
	}
	allEnabled := len(enabledLibraries) == 0

	libs := r.Libraries()
	infos := make([]LibraryInfo, 0, len(libs))
	for _, lib := range libs {
		infos = append(infos, LibraryInfo{
			Key:           lib.Key,
			Name:          lib.Name,
			License:       lib.License,
			Version:       lib.bundledVersion(),
			Enabled:       allEnabled || enabledSet[lib.Key],
			ResourceCount: len(lib.Resources),
		})
	}
	return infos
}

// bundledVersion returns the highest bundled version across the library's
// resources, or the first available version if none can be parsed.
func (l *Library) bundledVersion() string {
	var best semver.Version
	haveBest := false
	for _, res := range l.Resources {
		v, err := semver.ParseTolerant(res.mapping.Version)
		if err != nil {
			continue
		}
		if !haveBest || v.GT(best) {
			best = v
			haveBest = true
		}
	}
	if haveBest {
		return best.String()
	}
	if len(l.Resources) > 0 {
		return l.Resources[0].mapping.Version
	}
	return ""
}

// compileMapping compiles every pattern of a mapping into a resource.
func compileMapping(mapping config.LocalResourceMapping, lib *Library, custom bool) ([]*resource, error) {
	if len(mapping.Patterns) == 0 {
		return nil, errors.New("mapping has no URL patterns")
	}
	if mapping.File == "" {
		return nil, errors.New("mapping has no local file")
	}

	var versionRange semver.Range
	var err error
	if mapping.VersionRange != "" {
		versionRange, err = semver.ParseRange(mapping.VersionRange)
		if err != nil {
			return nil, fmt.Errorf("invalid version range %q: %w", mapping.VersionRange, err)
		}
	}

	resources := make([]*resource, 0, len(mapping.Patterns))
	for _, pattern := range mapping.Patterns {
		host, wildcardHost, segments, query, err := compilePattern(pattern)
		if err != nil {
			return nil, err
		}
		resources = append(resources, &resource{
			mapping:      mapping,
			library:      lib,
			custom:       custom,
			host:         host,
			wildcardHost: wildcardHost,
			segments:     segments,
			query:        query,
			versionRange: versionRange,
		})
	}
	return resources, nil
}

// compilePattern parses a URL pattern into a host and path segments.
//
// Supported syntax:
//   - "{version}" matches one path segment and captures it as a version;
//   - "*" as the last path segment matches the remainder of the path;
//   - "*" elsewhere matches exactly one path segment;
//   - a host starting with "*." matches any subdomain.
func compilePattern(pattern string) (host string, wildcardHost bool, segments []segment, query url.Values, err error) {
	u, err := url.Parse(pattern)
	if err != nil {
		return "", false, nil, nil, fmt.Errorf("parse pattern %q: %w", pattern, err)
	}
	host = strings.ToLower(u.Hostname())
	if host == "" {
		return "", false, nil, nil, fmt.Errorf("pattern %q has no host", pattern)
	}
	if strings.HasPrefix(host, "*.") {
		wildcardHost = true
		host = strings.TrimPrefix(host, "*.")
	}
	if u.RawQuery != "" {
		query = u.Query()
	}

	// Use u.Path, not u.EscapedPath: EscapedPath percent-encodes the
	// braces in "{version}", which would break pattern compilation.
	rawPath := u.Path
	if rawPath == "" {
		rawPath = "/"
	}
	parts := strings.Split(strings.TrimPrefix(rawPath, "/"), "/")
	segments = make([]segment, 0, len(parts))
	for i, part := range parts {
		switch {
		case part == "*" && i == len(parts)-1:
			segments = append(segments, segment{wildcard: true, greedy: true})
		case strings.Contains(part, "{version}"):
			idx := strings.Index(part, "{version}")
			segments = append(segments, segment{
				version: true,
				prefix:  part[:idx],
				suffix:  part[idx+len("{version}"):],
			})
		case strings.Contains(part, "*"):
			segments = append(segments, segment{wildcard: true})
		default:
			segments = append(segments, segment{literal: part})
		}
	}
	return host, wildcardHost, segments, query, nil
}

// match reports whether the resource matches the given request path and, if
// the pattern contains a {version} placeholder, returns the captured version.
func (res *resource) match(requestPath string) (version string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(requestPath, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		parts = nil
	}
	segs := res.segments
	for i := 0; i < len(parts); i++ {
		if i >= len(segs) {
			return "", false
		}
		seg := segs[i]
		switch {
		case seg.greedy:
			return version, true
		case seg.version:
			captured, ok := matchVersionSegment(parts[i], seg)
			if !ok {
				return "", false
			}
			version = captured
		case seg.wildcard:
			// Matches any single path segment.
		default:
			if parts[i] != seg.literal {
				return "", false
			}
		}
	}
	return version, len(parts) == len(segs)
}

// matchVersionSegment extracts the version captured by a {version} segment.
func matchVersionSegment(part string, seg segment) (string, bool) {
	if !strings.HasPrefix(part, seg.prefix) || !strings.HasSuffix(part, seg.suffix) {
		return "", false
	}
	version := strings.TrimSuffix(strings.TrimPrefix(part, seg.prefix), seg.suffix)
	if !isVersionLike(version) {
		return "", false
	}
	return version, true
}

// isVersionLike reports whether the string looks like a version number.
func isVersionLike(v string) bool {
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return false
	}
	return v[0] >= '0' && v[0] <= '9'
}

// versionInRange reports whether the captured version satisfies the resource's
// version range. Unparseable versions are treated as matching.
func (res *resource) versionInRange(version string) bool {
	if res.versionRange == nil {
		return true
	}
	v, err := semver.ParseTolerant(version)
	if err != nil {
		return true
	}
	return res.versionRange(v)
}

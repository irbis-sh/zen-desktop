# Local Resource Interception & Injection Engine

The Local Resource Interception & Injection Engine brings the
[LocalCDN](https://codeberg.org/nobody/LocalCDN) philosophy to Zen's proxy:
when a page requests a well-known library from a CDN, Zen serves a bundled
local copy instead of letting the request reach the remote CDN. Everything
happens inside Zen's proxy; no request is forwarded upstream for a matched
resource, and no data leaves the device.

The engine lives in `internal/localcdn`. It is a native Go re-implementation
of the interception logic; it does not import or depend on any LocalCDN code.

## Pipeline integration

The proxy (`internal/proxy`) calls a `filter` interface for every request.
`internal/app` wires a composite filter (`localcdnFilter`) that runs Zen's
existing ad-blocking `filter.Filter` first and the local resource engine
second:

1. `filter.Filter.HandleRequest` — if a filter-list rule blocks or redirects
   the request, that response wins and the local engine never runs.
2. `localcdn.Engine.HandleRequest` — if the URL matches a bundled resource,
   the local copy is returned with immutable caching headers. If
   "block missing resources" is enabled and the host is a known CDN without a
   local copy, a `204 No Content` response is returned.
3. `localcdn.Engine.HandleResponse` — HTML responses are rewritten to strip
   `integrity` and `crossorigin` attributes from matching `<script>`,
   `<link rel="stylesheet">`, and `<style>` tags, so browsers accept the
   locally served replacement. This runs before the existing script/style
   injection pipeline, and both rewrites compose through
   `internal/httprewrite`.

Requests served locally are emitted on Zen's `filter:action` event channel
with kind `local` (plus the library name), so the request log can show them as
"Local" instead of "Blocked" or "Allowed".

## Registry & pattern matching

`internal/localcdn/resources.json` is the machine-readable registry manifest
(schema in `internal/localcdn/registry.go`). It lists libraries, their bundled
resource files, URL patterns, version ranges, content types, and SHA-384 SRI
hashes.

Pattern syntax:

| Syntax | Meaning |
| --- | --- |
| `https://cdn.example.com/libs/{version}/app.min.js` | `{version}` matches one path segment and captures it as a version |
| `https://cdn.example.com/npm/pkg@*/dist/app.js` | `*` matches one path segment |
| `https://cdn.example.com/assets/*` | a trailing `*` matches the rest of the path |
| `https://*.cdn.example.com/...` | host wildcard matches any subdomain |
| `?family=Material+Icons` in a pattern | the request query must contain the same parameters |
| `versionRange` field | a [blang/semver](https://github.com/blang/semver) range, e.g. `>=3.0.0 <4.0.0` |

Lookup is a hash map keyed on hostname, and each host has a small bounded list
of compiled patterns, so matching is effectively O(1) per request. Version
parsing only happens for patterns that capture `{version}`.

## Embedded resources & custom mappings

Bundled library files are embedded into the binary with Go's `embed` package
(`internal/localcdn/embed.go`) under `internal/localcdn/resources/`. Update
them at build time with:

```sh
task localcdn:update-resources
```

which re-downloads the files from upstream CDNs
(`scripts/update-localcdn-resources.sh`) and regenerates
`resources.json` with fresh SRI hashes
(`scripts/update-localcdn-resources.ps1`).

Users can add their own replacements:

- Set a custom resource directory in Settings > Local Resources. Files are
  resolved relative to that directory (path traversal is rejected).
- Import/export custom resource mappings as JSON (a list of mapping objects
  with `patterns`, `file`, `contentType`, `version`, and optional
  `versionRange`/`sri`). Imported mappings are stored in Zen's config and
  loaded at the next proxy start.

## Exclusions & filter-list precedence

- The engine honors Zen's proxy exclusions: `sysproxy.IsExcludedHost` combines
  the built-in sensitive-host lists with the user's ignored hosts, and the
  engine passes those hosts through untouched.
- Filter-list blocks always take precedence over local serving (the composite
  filter returns the block response before the engine runs).
- Locally served requests are not counted as blocked requests; they get their
  own "Local" event kind.

## Statistics

The engine counts resources served since installation and since the last
reset, plus per-library and per-CDN-host breakdowns. Counters are thread-safe,
shared between the proxy engine and the Wails `localcdn.Manager`, and are
persisted to the config periodically and on proxy shutdown.

## Configuration & Wails bindings

Settings live in `config.LocalResources` (`config.json` → `localResources`).
The Wails-bound `App` methods (see `internal/app/localcdn.go`) expose:

- `GetLocalResourcesSettings` / `SetLocalResourcesEnabled` /
  `SetLocalResourcesBlockMissing` / `SetLocalResourcesCustomDir`
- `GetLocalResourcesLibraries` / `SetLocalResourcesLibraryEnabled`
- `GetLocalResourcesStats` / `ResetLocalResourcesStats`
- `ExportLocalResourcesMappings` / `ImportLocalResourcesMappings`

Frontend components live in
`frontend/src/components/settings/LocalResources/` and are rendered from the
Settings screen. All UI strings are localized in `frontend/src/i18n/locales/`.

## License notes

LocalCDN itself is GPL-3.0; Zen is MIT. The interception logic in
`internal/localcdn` is an original Go implementation and is MIT-licensed. The
bundled resource files are the upstream library files (jQuery, Bootstrap,
React, etc.), which are distributed under their own permissive licenses — see
[`THIRD-PARTY-NOTICES.md`](../../THIRD-PARTY-NOTICES.md) for details.

## Interaction with browser cache

Served resources carry
`Cache-Control: public, max-age=31536000, immutable`, so browsers cache them
aggressively. As a result, resources already present in the browser cache are
served from disk and never reach Zen's proxy — they are not intercepted and do
not increment the injection counter. This is intentional: the immutable cache
headers are the performance rationale for the module, and the counter tracks
actual interceptions (cache misses), not page views.

To observe interception on a previously visited site, clear the site's cached
data once (or use a private/incognito window). The first load then flows
through Zen, the local copy is served and counted, and the immutable headers
cache it for subsequent visits.

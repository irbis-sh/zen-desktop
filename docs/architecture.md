# Architecture

## Problem and central decision

For most people, an ad-blocker is a browser extension. That is the established path: the browser hands the extension a clean, sandboxed hook into every request. It's also a limitation. An extension cannot see an ad request from a desktop app, or telemetry from a native application or the operating system itself.

So Zen ships as a desktop app and filters traffic for the whole machine instead, sitting between applications and the network as a local HTTP/HTTPS proxy. Working at the HTTP layer gives it fine granularity - it can block a concrete path, strip a tracking parameter from a query string, or a field out of a JSON response body.

## The shape of the application

Zen is a single process: a Go core "backend" with a React UI "frontend", put together by [Wails](https://wails.io). The frontend lives in a WebView window and calls bound Go methods - start and stop the proxy, edit filter lists, change settings. The backend publishes events consumed by the frontend: proxy state changes, and a live stream of filter actions that feeds the request log. A tray icon and an autostart entry keep Zen running in the background on the platforms that support them.

Zen stores some data on disk: a JSON config file (filter list selection, custom rules, routing policy, settings), a data directory holding the certificate authority (described below), a cache of downloaded filter lists, and a log file.

## Getting traffic to the proxy

There are several ways to intercept a machine's traffic. Zen's choice is OS proxy settings and a plain HTTP/`CONNECT` forward proxy. Zen currently sets the OS proxy configuration using a [PAC (proxy auto-config)](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_PAC_file).

The choice of PAC over static proxy configuration is guided by Zen's exclusion list of hostnames, which includes banking, government services, login and identity providers. Static configuration's exception fields are small - around 2,000 characters on Windows, around 650 on macOS - while a PAC file can grow to about a megabyte.

PAC also brings a nice side effect: the script is itself fetched over HTTP, so Zen can see which process is requesting it and hand an excluded app an all-`DIRECT` script. Per-app allow-/block-listing comes nearly free.

The downside of this approach is that some traffic is missed. In practice every mainstream browser and most apps built on modern networking stacks honour the system proxy, but some do not. On Linux, only GNOME and KDE offer a system proxy setting at all. The planned medium-term answer is DNS-based blocking - coarse, domain-granular only, but it reaches apps that do not respect the system proxy or don't communicate via HTTP.

The long-term solution we want is full L3 capture (a TUN interface, NetworkExtension, a VPN service), so Zen would become the machine's default outbound route. A significant obstacle is that on Windows a TUN driver is a kernel driver, and signing one needs an EV certificate that's difficult for an open-source project to get.

## Intercepting TLS

A canonical proxy sees two types of traffic. Plain HTTP is simply relayed: the client puts the full URL in the request line, and the proxy makes the request on its behalf. For HTTPS the client instead sends `CONNECT example.com:443`, the proxy opens a TCP connection to that host, replies `200`, and from then on is supposed to transfer bytes transparently in both directions.

To read and modify HTTPS, Zen has to man-in-the-middle that connection. On first run Zen generates a root certificate authority locally and installs it into the system trust store (see [`internal/certstore`](/internal/certstore)). From then on it generates a short-lived leaf certificate per host, cached in an LRU. The CA key is generated locally, written to disk with `0600` permissions, and never leaves the device.

A leaked key of this kind would let an attacker impersonate any website to this machine, so the design works to limit the blast radius:

- Sensitive traffic is never proxied. The PAC exclusion list ([`internal/sysproxy/exclusions`](/internal/sysproxy/exclusions)) - banks, government portals, login and identity providers, plus platform-critical hosts (Microsoft, Apple) - routes those hosts `DIRECT`, so Zen never sees or terminates their TLS.
- When interception fails, Zen ignores the host. An example is certificate pinning - a pinned client rejects Zen's leaf outright. On a TLS error - in the handshake with the client or on the upstream connection - Zen adds the host to an in-memory passthrough set and tunnels it untouched from then on. Literal IP hosts are always tunnelled, since without a hostname there is nothing to put in a certificate.

This doesn't makes interception risk-free - it's a real liability, taken on because there is no other way to intercept HTTPS. Supply-chain security - including immutable releases, provenance attestations, and trust-store handling - is covered in [Security Architecture](internal/security-architecture.md).

## The life of a request

Suppose the browser asks for `https://ads.example.com/banner.js`. The PAC script has already routed the request to Zen, so it arrives to Zen's local listener, and four things happen in order:

1. **Attribute.** Zen maps the connection's source port back to the process that opened it - its PID and executable path - using per-OS lookups ([`internal/process`](/internal/process)). If the user's routing policy excludes that app, the connection is tunnelled untouched - a backstop for traffic that reaches the proxy despite the PAC exclusion (a cached PAC script, an app pointed at the proxy directly).
2. **Intercept.** Zen mints a leaf certificate for `ads.example.com`, completes the TLS handshake as if it were that host, and reads the request in plaintext, as described above. HTTP/1.1 and HTTP/2 are both handled; plain HTTP requests skip this step.
3. **Match.** The URL is tested against the loaded rules - hundreds of thousands of EasyList-style and hosts-format filters.
4. **Act.** On a block match, Zen answers instead of the actual server. A user navigation (recognised from the `Sec-Fetch-*` headers) gets a small HTML block page; a subresource like our `banner.js` gets a bare block response (403 Forbidden). On a modify match, or on no match at all, the request goes upstream - possibly with parameters stripped or headers rewritten - and the response is filtered the same way on the way back: headers modified, JSON bodies pruned, and HTML documents given the cosmetic-filtering payload described below.

WebSocket upgrades are filtered before the upgrade - so a rule can still block a `wss://` endpoint - and then tunnelled bidirectionally. Streaming responses (server-sent events, downloads of unknown length) are flushed write-by-write, so a live stream does not get buffered by the proxy.

## Matching

The match happens synchronously, on the critical path of every request from every app on the machine. Testing a URL against hundreds of thousands of rules one by one would cost O(rules) per request - too slow for something in front of the whole system.

So network rules live in a compressed token-based radix tree ([`internal/ruletree`](/internal/ruletree)). Rules are tokenised and inserted with path compression, so a match costs roughly as much as the URL is long. An off-the-shelf radix tree would not do: adblock patterns carry [special matching syntax](https://docs.irbis.sh/docs/zen/reference/filter-list-synax/network-rules/) - domain anchors, separator wildcards - so traversal carries a fair amount of domain-specific logic.

A second, simpler matcher ([`internal/hostmatch`](/internal/hostmatch)) handles rules keyed by hostname alone (the cosmetic and scriptlet rules below). Hostnames are a different enough problem - wildcards, exception lists, no paths - that they get their own structure.

The tree's main limitation is that reads and writes cannot overlap: it is built once, compacted, and then only queried. Changing the rule set means rebuilding, and we have not yet worked out how to let some threads traverse the tree while others mutate it.

## Acting beyond blocking

In addition to blocking, Zen supports:

- Request and response rewriting. Rule modifiers can strip individual query parameters, add or remove headers, and prune keys out of JSON response bodies.
- Cosmetic filtering and scriptlets. Element-hiding CSS, extended CSS and scriptlets - small JavaScript functions that neutralise trackers from inside the page - are injected into HTML documents ([`internal/asset`](/internal/asset)). Zen rewrites the document's `<head>` in flight, appending `<script>` and `<link rel="stylesheet">` tags, and patches the page's [`Content-Security-Policy`](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy) with fresh nonces so the page's own CSP policy does not block the injection.

The injected tags point at `https://local.irbis.sh/`, a fake hostname served by the proxy itself. Zen used to serve these assets from an HTTPS listener on 127.0.0.1, but Firefox's Local Network Access requires an inconvenient and misleading UI permission prompt before allowing requests to it. However, there's a tradeoff which we have to accept: with Zen switched off, a cached page referencing the name leaks a DNS query.

## Filter lists

Rules come from filter lists following the [established syntax](https://docs.irbis.sh/docs/zen/reference/filter-list-synax/), plus hosts lists and the user's own rules. The list store ([`internal/filterliststore`](/internal/filterliststore)) is built so that it comes up even when the network is down or slow.

Lists also carry a notion of trust. Most rules are safe to accept from any list, but raw CSS and raw JavaScript injections, and certain scriptlets are accepted only from a handful of trusted lists and from the user's own rules. The policy for granting a list trusted status lives in [Filter Lists](internal/filter-lists.md).

## Self-update

Zen updates itself ([`internal/selfupdate`](/internal/selfupdate)). It periodically fetches a small per-OS, per-architecture manifest JSON from `update-manifests.zenprivacy.net`, compares versions, downloads the release asset, and verifies its SHA-256 against the manifest before swapping the executable. Builds distributed through package managers have self-update turned off and defer to the package manager instead.

package sysproxy

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
)

var (
	pacTemplate = template.Must(
		template.New("pac").Parse(`function FindProxyForURL(url, host) {
	var userExcludedHosts = [{{range $i, $h := .UserExcludedHosts}}{{if $i}},{{end}}"{{$h}}"{{end}}];
	for (var i = 0; i < userExcludedHosts.length; i++) {
		if (dnsDomainIs(host, userExcludedHosts[i])) {
			return "DIRECT";
		}
	}
	var cdnHosts = [{{range $i, $h := .CDNHosts}}{{if $i}},{{end}}"{{$h}}"{{end}}];
	for (var i = 0; i < cdnHosts.length; i++) {
		if (host === cdnHosts[i]) {
			return "PROXY 127.0.0.1:{{.ProxyPort}}; DIRECT";
		}
	}
	var excludedHosts = [{{range $index, $host := .ExcludedHosts}}{{if $index}},{{end}}"{{$host}}"{{end}}];
	for (var i = 0; i < excludedHosts.length; i++) {
		if (dnsDomainIs(host, excludedHosts[i])) {
			return "DIRECT";
		}
	}
	return "PROXY 127.0.0.1:{{.ProxyPort}}; DIRECT";
}`))
	transparentPAC = []byte(`function FindProxyForURL(url, host) { return "DIRECT"; }`)

	//go:embed exclusions/common.txt
	commonExcludedHosts []byte
)

// renderPac returns the PAC file content for the given proxy port,
// user-configured excluded hosts, and CDN hosts that must always be routed
// through the proxy (so bundled local resources can be intercepted even when a
// parent domain of the CDN is excluded, e.g. cdnjs.cloudflare.com under the
// cloudflare.com exclusion). User-configured exclusions take precedence over
// the CDN override.
func renderPac(proxyPort int, userExcludedHosts []string, cdnHosts []string) []byte {
	var buf bytes.Buffer
	pacTemplate.Execute(&buf, struct {
		ProxyPort         int
		UserExcludedHosts []string
		CDNHosts          []string
		ExcludedHosts     []string
	}{
		ProxyPort:         proxyPort,
		UserExcludedHosts: userExcludedHosts,
		CDNHosts:          cdnHosts,
		ExcludedHosts:     builtInExcludedHosts(),
	})
	return buf.Bytes()
}

// builtInExcludedHosts returns the exclusion hosts shipped with Zen (common
// plus platform-specific), without any user-configured hosts.
func builtInExcludedHosts() []string {
	var excludedHosts []string
	processList := func(data []byte) {
		for _, line := range bytes.Split(data, []byte("\n")) {
			if hashIndex := bytes.IndexByte(line, '#'); hashIndex != -1 {
				line = line[:hashIndex]
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			excludedHosts = append(excludedHosts, string(line))
		}
	}
	processList(commonExcludedHosts)
	processList(platformSpecificExcludedHosts)
	return excludedHosts
}

// buildExcludedHosts returns a list of hosts that should be excluded from being proxied.
// It combines common, platform-specific, and user-configured excluded hosts.
func buildExcludedHosts(userConfiguredExcludedHosts []string) []string {
	excludedHosts := builtInExcludedHosts()
	excludedHosts = append(excludedHosts, userConfiguredExcludedHosts...)

	return excludedHosts
}

// IsExcludedHost reports whether the given host (or a parent domain of it) is
// excluded from proxying, either by Zen's built-in exclusion lists or by the
// user-configured ignored hosts.
func IsExcludedHost(host string, userConfiguredExcludedHosts []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}

	for _, excludedHost := range buildExcludedHosts(userConfiguredExcludedHosts) {
		excludedHost = strings.ToLower(strings.TrimSpace(excludedHost))
		if excludedHost == "" {
			continue
		}
		if host == excludedHost || strings.HasSuffix(host, "."+excludedHost) {
			return true
		}
	}

	return false
}

// IsUserExcludedHost reports whether the given host (or a parent domain of it)
// is in the user-configured ignored hosts list. User-configured exclusions
// take precedence over the local resource engine's CDN host override.
func IsUserExcludedHost(host string, userConfiguredExcludedHosts []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}

	for _, excludedHost := range userConfiguredExcludedHosts {
		excludedHost = strings.ToLower(strings.TrimSpace(excludedHost))
		if excludedHost == "" {
			continue
		}
		if host == excludedHost || strings.HasSuffix(host, "."+excludedHost) {
			return true
		}
	}
	return false
}

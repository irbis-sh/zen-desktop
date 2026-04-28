package sysproxy

import (
	"bytes"
	_ "embed"
	"text/template"
)

var (
	pacTemplate = template.Must(
		template.New("pac").Parse(`function FindProxyForURL(url, host) {
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

// renderPac returns the PAC file content for the given proxy port and user-configured excluded hosts.
func renderPac(proxyPort int, userConfiguredExcludedHosts []string) []byte {
	var buf bytes.Buffer
	pacTemplate.Execute(&buf, struct {
		ProxyPort     int
		ExcludedHosts []string
	}{
		ProxyPort:     proxyPort,
		ExcludedHosts: buildExcludedHosts(userConfiguredExcludedHosts),
	})
	return buf.Bytes()
}

// buildExcludedHosts returns a list of hosts that should be excluded from being proxied.
// It combines common, platform-specific, and user-configured excluded hosts.
func buildExcludedHosts(userConfiguredExcludedHosts []string) []string {
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
	excludedHosts = append(excludedHosts, userConfiguredExcludedHosts...)

	return excludedHosts
}

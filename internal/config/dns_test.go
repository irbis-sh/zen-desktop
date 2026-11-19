package config

import (
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestDNSPresetsAreUsable(t *testing.T) {
	t.Parallel()

	for server, preset := range dnsPresets {
		u, err := url.Parse(preset.url)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			t.Errorf("%s: URL %q isn't https with a host", server, preset.url)
		}
		// The DoH client tries IPv4 first, and many networks have no IPv6 route.
		if !slices.ContainsFunc(preset.bootstrap, func(a netip.Addr) bool { return a.Is4() }) {
			t.Errorf("%s: no IPv4 address", server)
		}
	}
}

func TestDoHServer(t *testing.T) {
	t.Parallel()

	const customURL = "https://dns.example/dns-query"
	quad9 := dnsPresets[DNSServerQuad9]
	tests := []struct {
		name          string
		config        DNSConfig
		wantURL       string
		wantBootstrap []netip.Addr
		wantOK        bool
	}{
		{"empty", DNSConfig{}, "", nil, false},
		{"system", DNSConfig{Server: DNSServerSystem, CustomURL: customURL}, "", nil, false},
		{"preset", DNSConfig{Server: DNSServerQuad9, CustomURL: customURL}, quad9.url, quad9.bootstrap, true},
		{"custom", DNSConfig{Server: DNSServerCustom, CustomURL: customURL}, customURL, nil, true},
		{"custom without URL", DNSConfig{Server: DNSServerCustom}, "", nil, false},
		{"unknown", DNSConfig{Server: "quad10", CustomURL: customURL}, "", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotURL, bootstrap, ok := tt.config.DoHServer()
			if gotURL != tt.wantURL || !slices.Equal(bootstrap, tt.wantBootstrap) || ok != tt.wantOK {
				t.Errorf("DoHServer() = %q, %v, %v; want %q, %v, %v", gotURL, bootstrap, ok, tt.wantURL, tt.wantBootstrap, tt.wantOK)
			}
		})
	}
}

// TestConfigWithoutDNSUsesOS checks that existing users stay on the OS resolver: their
// configs, like the default one, have no "dns" key.
func TestConfigWithoutDNSUsesOS(t *testing.T) {
	c := newTestConfig(t)
	if _, _, ok := c.GetDNS().DoHServer(); ok {
		t.Error("DoHServer() ok = true, want false")
	}
}

func TestDNSServerEnumMatchesPresets(t *testing.T) {
	t.Parallel()

	inEnum := make(map[DNSServer]bool, len(DNSServerEnum))
	for _, s := range DNSServerEnum {
		inEnum[s.Value] = true
		if s.Value == DNSServerSystem || s.Value == DNSServerCustom {
			continue
		}
		if _, ok := dnsPresets[s.Value]; !ok {
			t.Errorf("%s is in DNSServerEnum but has no preset", s.Value)
		}
	}
	for server := range dnsPresets {
		if !inEnum[server] {
			t.Errorf("preset %s isn't in DNSServerEnum", server)
		}
	}
}

func TestSetDNSServer(t *testing.T) {
	c := newTestConfig(t)

	for _, s := range DNSServerEnum {
		if err := c.SetDNSServer(s.Value); err != nil {
			t.Fatalf("SetDNSServer(%q): %v", s.Value, err)
		}
		if got := c.GetDNS().Server; got != s.Value {
			t.Errorf("after SetDNSServer(%q), Server = %q", s.Value, got)
		}
	}

	if err := c.SetDNSServer(DNSServerQuad9); err != nil {
		t.Fatalf("SetDNSServer(%q): %v", DNSServerQuad9, err)
	}
	for _, server := range []DNSServer{"quad10", ""} {
		if err := c.SetDNSServer(server); err == nil {
			t.Errorf("SetDNSServer(%q) = nil, want an error", server)
		}
		if got := c.GetDNS().Server; got != DNSServerQuad9 {
			t.Errorf("after SetDNSServer(%q), Server = %q, want %q", server, got, DNSServerQuad9)
		}
	}
}

func TestSetDNSCustomURL(t *testing.T) {
	const earlier = "https://earlier.example/dns-query"
	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{"plain", "https://dns.example/dns-query", "https://dns.example/dns-query", false},
		{"port", "https://dns.example:8443/q", "https://dns.example:8443/q", false},
		{"IP address", "https://192.0.2.1/dns-query", "https://192.0.2.1/dns-query", false},
		{"template and spaces", " https://dns.example/dns-query{?dns} ", "https://dns.example/dns-query", false},
		{"empty", "", "", false},
		{"http", "http://dns.example/dns-query", earlier, true},
		{"no scheme", "dns.example/dns-query", earlier, true},
		{"scheme only", "https://", earlier, true},
		{"no host", "https:///dns-query", earlier, true},
		{"bad escape", "https://dns.example/%zz", earlier, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestConfig(t)
			if err := c.SetDNSCustomURL(earlier); err != nil {
				t.Fatalf("SetDNSCustomURL(%q): %v", earlier, err)
			}

			err := c.SetDNSCustomURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetDNSCustomURL(%q) error = %v, want error %v", tt.rawURL, err, tt.wantErr)
			}
			// For "bad escape", url.Parse's own error would quote the URL.
			if err != nil && strings.Contains(err.Error(), "dns.example") {
				t.Errorf("SetDNSCustomURL(%q) error = %q, which quotes the URL", tt.rawURL, err)
			}
			if got := c.GetDNS().CustomURL; got != tt.want {
				t.Errorf("after SetDNSCustomURL(%q), CustomURL = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestSetDNSSystemDomainsNormalises(t *testing.T) {
	c := newTestConfig(t)

	domains := []string{" *.Corp.Example. ", ".lab.example", "corp.example", "", "LAB.example"}
	if err := c.SetDNSSystemDomains(domains); err != nil {
		t.Fatalf("SetDNSSystemDomains: %v", err)
	}
	want := []string{"corp.example", "lab.example"}
	if got := c.GetDNS().SystemDomains; !slices.Equal(got, want) {
		t.Errorf("SystemDomains = %q, want %q", got, want)
	}
}

func TestExportDebugDataDNSServer(t *testing.T) {
	tests := []struct {
		name   string
		server DNSServer
		want   string
	}{
		{"preset", DNSServerQuad9, `"dnsServer": "quad9"`},
		{"no dns key", "", `"dnsServer": "system"`},
		{"unknown", "https://dns.nextdns.io/abc123", `"dnsServer": "unknown"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestConfig(t)
			c.DNS.Server = tt.server

			got, err := c.ExportDebugData()
			if err != nil {
				t.Fatalf("ExportDebugData: %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("ExportDebugData() = %s, want it to contain %s", got, tt.want)
			}
			if strings.Contains(got, "nextdns") {
				t.Errorf("ExportDebugData() = %s, which contains the server value", got)
			}
		})
	}
}

func TestExportDebugDataOmitsDNSDetails(t *testing.T) {
	c := newTestConfig(t)
	if err := c.SetDNSServer(DNSServerCustom); err != nil {
		t.Fatalf("SetDNSServer: %v", err)
	}
	if err := c.SetDNSCustomURL("https://dns.nextdns.io/abc123"); err != nil {
		t.Fatalf("SetDNSCustomURL: %v", err)
	}
	if err := c.SetDNSSystemDomains([]string{"corp.example"}); err != nil {
		t.Fatalf("SetDNSSystemDomains: %v", err)
	}

	got, err := c.ExportDebugData()
	if err != nil {
		t.Fatalf("ExportDebugData: %v", err)
	}
	if !strings.Contains(got, `"dnsServer": "custom"`) {
		t.Errorf("ExportDebugData() = %s, want dnsServer custom", got)
	}
	for _, secret := range []string{"nextdns", "corp.example"} {
		if strings.Contains(got, secret) {
			t.Errorf("ExportDebugData() = %s, which contains %q", got, secret)
		}
	}
}

func TestSetDNSServerSurvivesReload(t *testing.T) {
	c := newTestConfig(t)
	if err := c.SetDNSServer(DNSServerQuad9); err != nil {
		t.Fatalf("SetDNSServer: %v", err)
	}

	reloaded, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := reloaded.GetDNS().Server; got != DNSServerQuad9 {
		t.Errorf("after reload, Server = %q, want %q", got, DNSServerQuad9)
	}
}

// newTestConfig points ConfigDir at a temporary directory for the rest of the test and
// returns a config created there.
func newTestConfig(t *testing.T) *Config {
	t.Helper()
	prevConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() { ConfigDir = prevConfigDir })

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

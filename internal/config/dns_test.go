package config

import (
	"net/netip"
	"net/url"
	"slices"
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
	prevConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() { ConfigDir = prevConfigDir })

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, _, ok := c.GetDNS().DoHServer(); ok {
		t.Error("DoHServer() ok = true, want false")
	}
}

package config

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

// DNSServer selects how the proxy resolves upstream hosts: through the OS, or through a
// DNS-over-HTTPS (DoH) server.
type DNSServer string

const (
	DNSServerSystem     DNSServer = "system"
	DNSServerQuad9      DNSServer = "quad9"
	DNSServerQuad9ECS   DNSServer = "quad9ecs"
	DNSServerCloudflare DNSServer = "cloudflare"
	DNSServerGoogle     DNSServer = "google"
	DNSServerOpenDNS    DNSServer = "opendns"
	DNSServerCustom     DNSServer = "custom"
)

// DNSServerEnum binds DNSServer to the frontend.
var DNSServerEnum = []struct {
	Value  DNSServer
	TSName string
}{
	{DNSServerSystem, "SYSTEM"},
	{DNSServerQuad9, "QUAD9"},
	{DNSServerQuad9ECS, "QUAD9_ECS"},
	{DNSServerCloudflare, "CLOUDFLARE"},
	{DNSServerGoogle, "GOOGLE"},
	{DNSServerOpenDNS, "OPENDNS"},
	{DNSServerCustom, "CUSTOM"},
}

type dohPreset struct {
	url string
	// bootstrap holds the addresses to dial, so that connecting to the DoH server
	// doesn't need a lookup through the OS.
	bootstrap []netip.Addr
}

// dnsPresets maps each preset DNSServer to its DoH server. The servers were checked on
// 2026-11-10: every IPv4 address answered a DoH query with the URL's host as the TLS
// server name. The IPv6 addresses are the ones each provider publishes in DNS, and DoH
// over them is unchecked.
var dnsPresets = map[DNSServer]dohPreset{
	DNSServerQuad9: {
		url:       "https://dns.quad9.net/dns-query",
		bootstrap: addrs("9.9.9.9", "149.112.112.112", "2620:fe::fe", "2620:fe::9"),
	},
	DNSServerQuad9ECS: {
		url:       "https://dns11.quad9.net/dns-query",
		bootstrap: addrs("9.9.9.11", "149.112.112.11", "2620:fe::11", "2620:fe::fe:11"),
	},
	DNSServerCloudflare: {
		url: "https://cloudflare-dns.com/dns-query",
		bootstrap: addrs("1.1.1.1", "1.0.0.1", "104.16.248.249", "104.16.249.249",
			"2606:4700:4700::1111", "2606:4700:4700::1001"),
	},
	DNSServerGoogle: {
		url:       "https://dns.google/dns-query",
		bootstrap: addrs("8.8.8.8", "8.8.4.4", "2001:4860:4860::8888", "2001:4860:4860::8844"),
	},
	DNSServerOpenDNS: {
		url:       "https://doh.opendns.com/dns-query",
		bootstrap: addrs("208.67.222.222", "208.67.220.220", "2620:119:35::35", "2620:119:53::53"),
	},
}

func addrs(s ...string) []netip.Addr {
	a := make([]netip.Addr, len(s))
	for i, v := range s {
		a[i] = netip.MustParseAddr(v)
	}
	return a
}

// DNSConfig holds the DNS settings.
type DNSConfig struct {
	// Server is DNSServerSystem when empty.
	Server DNSServer `json:"server"`
	// CustomURL is the DoH URL for DNSServerCustom. It is kept when another server is
	// selected.
	CustomURL string `json:"customURL"`
	// SystemDomains are the domains whose names always go to the OS resolver: lower-cased,
	// without a leading "*." or "." and without a trailing dot.
	SystemDomains []string `json:"systemDomains"`
}

// DoHServer returns the DoH server d selects: its URL and the addresses to dial. ok is
// false if d selects the OS resolver, which includes an unknown Server and
// DNSServerCustom without a URL. A custom server has no addresses: the DoH client looks
// up the URL's host through the OS.
func (d DNSConfig) DoHServer() (url string, bootstrap []netip.Addr, ok bool) {
	if d.Server == DNSServerCustom {
		return d.CustomURL, nil, d.CustomURL != ""
	}
	preset, ok := dnsPresets[d.Server]
	return preset.url, preset.bootstrap, ok
}

// SetDNSServer rejects a server outside DNSServerEnum, the empty one included, which only
// a config without a "dns" key holds.
func (c *Config) SetDNSServer(server DNSServer) error {
	if !isKnownDNSServer(server) {
		return errors.New("unknown DNS server")
	}
	return c.update(func() error {
		c.DNS.Server = server
		return nil
	})
}

// SetDNSCustomURL sets the custom server's DoH URL, or clears it if rawURL is empty. It
// removes a trailing "{?dns}", the RFC 8484 URI template that providers publish with
// their URLs.
func (c *Config) SetDNSCustomURL(rawURL string) error {
	rawURL = strings.TrimSuffix(strings.TrimSpace(rawURL), "{?dns}")
	// Checked before update, which logs every error, because the settings page saves
	// while the user types. The error never quotes the URL, which can hold an account
	// ID, as NextDNS's do. That rules out url.Parse's own error.
	if rawURL != "" {
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" {
			return errors.New("not an https URL with a host")
		}
	}
	return c.update(func() error {
		c.DNS.CustomURL = rawURL
		return nil
	})
}

// SetDNSSystemDomains sets the domains whose names always go to the OS resolver. It
// stores them in the form SystemDomains describes, without empty and repeated entries.
// It rejects nothing: an entry that isn't a domain never matches a name.
func (c *Config) SetDNSSystemDomains(domains []string) error {
	normalised := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, "*.")
		d = strings.TrimPrefix(d, ".")
		d = strings.TrimSuffix(d, ".")
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		normalised = append(normalised, d)
	}
	return c.update(func() error {
		c.DNS.SystemDomains = normalised
		return nil
	})
}

func isKnownDNSServer(server DNSServer) bool {
	for _, s := range DNSServerEnum {
		if s.Value == server {
			return true
		}
	}
	return false
}

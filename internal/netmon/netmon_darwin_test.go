package netmon

import (
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"howett.net/plist"
)

const serviceKey = "State:/Network/Service/0C5E6F2A-1B3D-4E5F-8A9B-123456789ABC/DNS"

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		values      map[string]any
		fileDomains []string
		tunnel      bool
		wantDomains []string
		wantVPN     bool
	}{
		{
			name: "servers only",
			values: map[string]any{
				globalDNSKey: dict("ServerAddresses", list("9.9.9.9", "1.1.1.1")),
				serviceKey:   dict("ServerAddresses", list("9.9.9.9", "1.1.1.1")),
			},
		},
		{
			name: "search domain",
			values: map[string]any{
				globalDNSKey: dict("SearchDomains", list("corp.example"), "ServerAddresses", list("9.9.9.9")),
			},
			wantDomains: []string{"corp.example"},
		},
		{
			name:        "DHCP domain name",
			values:      map[string]any{serviceKey: dict("DomainName", "dhcp.example")},
			wantDomains: []string{"dhcp.example"},
		},
		{
			name: "match domain",
			values: map[string]any{
				serviceKey: dict("SupplementalMatchDomains", list("vpn.example"), "ServerAddresses", list("10.0.0.53")),
			},
			wantDomains: []string{"vpn.example"},
		},
		{
			name: "empty match domain with servers",
			values: map[string]any{
				serviceKey: dict("SupplementalMatchDomains", list("", "Tail.Example."), "ServerAddresses", list("100.100.100.100")),
			},
			wantDomains: []string{"tail.example"},
			wantVPN:     true,
		},
		{
			name:   "empty match domain without servers",
			values: map[string]any{serviceKey: dict("SupplementalMatchDomains", list(""))},
		},
		{
			name: "match domain of two dots",
			values: map[string]any{
				serviceKey: dict("SupplementalMatchDomains", list(".."), "ServerAddresses", list("10.0.0.53")),
			},
		},
		{
			name: "search domains as a string",
			values: map[string]any{
				serviceKey:   dict("SearchDomains", "bad.example"),
				globalDNSKey: dict("SearchDomains", list("corp.example")),
			},
			wantDomains: []string{"corp.example"},
		},
		{
			name: "value that isn't a dictionary",
			values: map[string]any{
				serviceKey:   "bad.example",
				globalDNSKey: dict("SearchDomains", list("corp.example")),
			},
			wantDomains: []string{"corp.example"},
		},
		{
			name:        "resolver file domains",
			fileDomains: []string{"Dev.Example.."},
			wantDomains: []string{"dev.example"},
		},
		{
			name:    "first server behind a tunnel",
			values:  map[string]any{globalDNSKey: dict("ServerAddresses", list("10.99.0.2"))},
			tunnel:  true,
			wantVPN: true,
		},
		{
			name:    "IPv4 loopback server",
			values:  map[string]any{globalDNSKey: dict("ServerAddresses", list("127.0.2.2"))},
			wantVPN: true,
		},
		{
			name:    "IPv6 loopback server",
			values:  map[string]any{globalDNSKey: dict("ServerAddresses", list("::1"))},
			wantVPN: true,
		},
		{
			name:   "first server not an address",
			values: map[string]any{globalDNSKey: dict("ServerAddresses", list("dns.example"))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parse(tt.values, tt.fileDomains, func(netip.Addr) bool { return tt.tunnel })
			slices.Sort(got.Domains)
			if !slices.Equal(got.Domains, tt.wantDomains) {
				t.Errorf("domains %q, want %q", got.Domains, tt.wantDomains)
			}
			if got.VPNHandlesAllDNS != tt.wantVPN {
				t.Errorf("VPNHandlesAllDNS = %v, want %v", got.VPNHandlesAllDNS, tt.wantVPN)
			}
		})
	}
}

func TestParseAsksRouteOnlyForAddresses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server string
		want   bool
	}{
		{name: "public address", server: "9.9.9.9", want: true},
		{name: "loopback address", server: "127.0.0.1", want: false},
		{name: "not an address", server: "dns.example", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			asked := false
			values := map[string]any{globalDNSKey: dict("ServerAddresses", list(tt.server))}
			parse(values, nil, func(netip.Addr) bool {
				asked = true
				return false
			})
			if asked != tt.want {
				t.Errorf("viaTunnel called = %v, want %v", asked, tt.want)
			}
		})
	}
}

// TestParseDecodedStore checks that parse accepts the types the decoder produces. Its
// type checks skip everything else, so a decoder that changed them would leave the state
// empty without an error.
func TestParseDecodedStore(t *testing.T) {
	t.Parallel()
	data, err := plist.Marshal(map[string]any{
		serviceKey: map[string]any{
			"SearchDomains":            []string{"corp.example"},
			"SupplementalMatchDomains": []string{"", "vpn.example"},
			"ServerAddresses":          []string{"10.0.0.53"},
		},
	}, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	values, err := decodeStore(data)
	if err != nil {
		t.Fatalf("decodeStore: %v", err)
	}
	got := parse(values, nil, func(netip.Addr) bool { return false })
	slices.Sort(got.Domains)
	want := State{Domains: []string{"corp.example", "vpn.example"}, VPNHandlesAllDNS: true}
	if !got.equal(want) {
		t.Errorf("parse(decoded) = %+v, want %+v", got, want)
	}
}

func TestResolverDomains(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		"dev.example": "nameserver 127.0.0.1\n",
		"vpn-a":       "nameserver 10.0.0.53\ndomain corp.example\n",
		"twice":       "domain first.example\ndomain last.example",
		"upper":       "DOMAIN upper.example\n",
		"tab":         "domain\ttab.example\n",
		"indented":    " domain wrong.example\nnameserver 10.0.0.53\n",
		"commented":   "# domain wrong.example\nnameserver 10.0.0.53\n",
		".DS_Store":   "domain hidden.example\n",
		"unreadable":  "domain unread.example\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(dir, "unreadable"), 0); err != nil {
		t.Fatal(err)
	}
	unreadable := "unreadable"
	if os.Geteuid() == 0 {
		// Root reads the file regardless of its mode.
		unreadable = "unread.example"
	}
	got := resolverDomains(dir)
	slices.Sort(got)
	want := []string{
		"commented", "corp.example", "dev.example", "first.example", "indented", "tab.example", unreadable, "upper.example",
	}
	if !slices.Equal(got, want) {
		t.Errorf("resolverDomains = %q, want %q", got, want)
	}

	if got := resolverDomains(filepath.Join(dir, "missing")); got != nil {
		t.Errorf("resolverDomains(missing directory) = %q, want none", got)
	}
}

// The tests below use the real system. They can't change it, which takes root.

func TestReadStateOnSystem(t *testing.T) {
	t.Parallel()
	if _, err := readState(); err != nil {
		t.Fatalf("readState: %v", err)
	}
}

func TestWatchAndStopOnSystem(t *testing.T) {
	t.Parallel()
	for range 100 {
		stop, err := watch(func() {})
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		stop()
	}
}

func TestRouteLookupOnSystem(t *testing.T) {
	t.Parallel()
	index, err := routeInterface(netip.MustParseAddr("127.0.0.1"))
	if err != nil {
		t.Fatalf("routeInterface(127.0.0.1): %v", err)
	}
	if viaTunnel(netip.MustParseAddr("127.0.0.1")) {
		t.Errorf("viaTunnel(127.0.0.1) = true, want false (interface %d)", index)
	}
}

// dict builds a store dictionary from alternating keys and values.
func dict(kv ...any) map[string]any {
	d := make(map[string]any, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		d[kv[i].(string)] = kv[i+1]
	}
	return d
}

// list builds an array as the property-list decoder returns one.
func list(items ...string) []any {
	l := make([]any, len(items))
	for i, item := range items {
		l[i] = item
	}
	return l
}

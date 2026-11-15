package netmon

import (
	"errors"
	"net/netip"
	"slices"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestParse(t *testing.T) {
	t.Parallel()

	upAdapter := func(ifType uint32, servers ...string) adapter {
		return adapter{up: true, ifType: ifType, servers: addrs(servers...)}
	}
	ethernet := upAdapter(windows.IF_TYPE_ETHERNET_CSMACD, "10.0.2.3")
	withSuffixes := func(a adapter, suffixes ...string) adapter {
		a.suffixes = suffixes
		return a
	}
	rule := func(namespace string, servers ...string) nrptRule {
		return nrptRule{namespaces: []string{namespace}, servers: servers}
	}

	tests := []struct {
		name        string
		sys         system
		wantDomains []string
		wantVPN     bool
	}{
		{name: "ethernet with a server", sys: system{adapters: []adapter{ethernet}}},
		{
			name:        "suffix lower-cased without its dot",
			sys:         system{adapters: []adapter{withSuffixes(ethernet, "Corp.Example.")}},
			wantDomains: []string{"corp.example"},
		},
		{
			name:        "repeated suffixes",
			sys:         system{adapters: []adapter{withSuffixes(ethernet, "tail.example", "tail.example", "other.example")}},
			wantDomains: []string{"other.example", "tail.example"},
		},
		{
			name: "adapter down",
			sys: system{adapters: []adapter{{
				ifType:   windows.IF_TYPE_ETHERNET_CSMACD,
				suffixes: []string{"corp.example"},
				servers:  addrs("127.0.0.1"),
			}}},
		},
		{
			name: "loopback interface",
			sys: system{adapters: []adapter{
				upAdapter(windows.IF_TYPE_SOFTWARE_LOOPBACK, "fec0:0:0:ffff::1", "fec0:0:0:ffff::2", "fec0:0:0:ffff::3"),
			}},
		},
		{name: "root rule with a space for servers", sys: system{rules: []nrptRule{rule(".", " ")}}},
		{
			name: "rules with servers and an empty or space name",
			sys:  system{rules: []nrptRule{rule("", "10.0.0.53"), rule(" ", "10.0.0.53")}},
		},
		{name: "primary suffix", sys: system{primarySuffix: "primary.example"}, wantDomains: []string{"primary.example"}},
		{
			name:        "search lists",
			sys:         system{searchLists: []string{"a.example,b.example", "c.example d.example"}},
			wantDomains: []string{"a.example", "b.example", "c.example", "d.example"},
		},
		{
			name:        "suffix rule with servers",
			sys:         system{rules: []nrptRule{rule(".vpn.example", "10.0.0.53")}},
			wantDomains: []string{"vpn.example"},
		},
		{
			name:        "suffix rule without servers",
			sys:         system{rules: []nrptRule{rule(".vpn.example", "", "")}},
			wantDomains: []string{"vpn.example"},
		},
		{
			name:        "host rule",
			sys:         system{rules: []nrptRule{rule("host.vpn.example", "10.0.0.53")}},
			wantDomains: []string{"host.vpn.example"},
		},
		{name: "root rule with servers", sys: system{rules: []nrptRule{rule(".", "10.0.0.53")}}, wantVPN: true},
		{name: "root rule in the second server value", sys: system{rules: []nrptRule{rule(".", "", "10.0.0.53")}}, wantVPN: true},
		{name: "root rule without servers", sys: system{rules: []nrptRule{rule(".", "", "")}}},
		{name: "proprietary virtual adapter", sys: system{adapters: []adapter{upAdapter(ifTypePropVirtual, "10.0.0.53")}}, wantVPN: true},
		{name: "PPP adapter", sys: system{adapters: []adapter{upAdapter(windows.IF_TYPE_PPP, "10.0.0.53")}}, wantVPN: true},
		{name: "tunnel adapter", sys: system{adapters: []adapter{upAdapter(windows.IF_TYPE_TUNNEL, "10.0.0.53")}}, wantVPN: true},
		{
			name: "VPN adapter with only a placeholder server",
			sys:  system{adapters: []adapter{upAdapter(ifTypePropVirtual, "fec0:0:0:ffff::1")}},
		},
		{name: "VPN adapter without servers", sys: system{adapters: []adapter{upAdapter(ifTypePropVirtual)}}},
		{
			name:    "loopback server after another",
			sys:     system{adapters: []adapter{upAdapter(windows.IF_TYPE_ETHERNET_CSMACD, "fe80::1", "127.0.0.1")}},
			wantVPN: true,
		},
		{
			name:    "IPv6 loopback server",
			sys:     system{adapters: []adapter{upAdapter(windows.IF_TYPE_ETHERNET_CSMACD, "::1")}},
			wantVPN: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parse(tt.sys)
			// The shared part sorts the domains and drops duplicates.
			slices.Sort(got.Domains)
			got.Domains = slices.Compact(got.Domains)
			if !slices.Equal(got.Domains, tt.wantDomains) || got.VPNHandlesAllDNS != tt.wantVPN {
				t.Errorf("parse = %q %v, want %q %v", got.Domains, got.VPNHandlesAllDNS, tt.wantDomains, tt.wantVPN)
			}
		})
	}
}

func TestReadStateWorks(t *testing.T) {
	t.Parallel()
	if _, err := readState(); err != nil {
		t.Fatal(err)
	}
}

// TestWatchStopRepeated catches crashes and deadlocks, not a callback after stop.
func TestWatchStopRepeated(t *testing.T) {
	t.Parallel()
	for range 100 {
		stop, err := watch(func() {}, func() {})
		if err != nil {
			t.Fatal(err)
		}
		stop()
	}
}

// TestCallbackGetsWatcher checks the callback's signature and its context pointer. With
// the initial notification, Windows calls the callback before the function returns,
// once for each address family.
func TestCallbackGetsWatcher(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	w := &watcher{network: func() { calls.Add(1) }}
	w.pinner.Pin(w)
	defer w.pinner.Unpin()

	var h windows.Handle
	if err := windows.NotifyIpInterfaceChange(windows.AF_UNSPEC, changedCallback, unsafe.Pointer(w), true, &h); err != nil {
		t.Fatal(err)
	}
	if err := windows.CancelMibChangeNotify2(h); err != nil {
		t.Fatal(err)
	}
	if calls.Load() == 0 {
		t.Error("callback not called")
	}
}

func TestRegistryChangeReachesSettings(t *testing.T) {
	t.Parallel()
	called := make(chan struct{}, 1)
	// Only the policy key, because Windows writes under the others by itself, and such a
	// write would pass the test even with this key's watch broken.
	stop := watchKeys([]string{policiesKey}, func() {
		select {
		case called <- struct{}{}:
		default:
		}
	})
	defer stop()

	// Windows doesn't read this key. It sits right under the watched key: under
	// Windows NT, the test would create that parent on a PC without it, and leave it
	// behind empty.
	const path = policiesKey + `\zen-netmon-test`
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, path, registry.SET_VALUE)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Skip("creating a key under HKLM needs administrator rights")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.LOCAL_MACHINE, path)
	// A key left behind by an interrupted run isn't created again, but this still changes it.
	err = k.SetStringValue("test", "1")
	k.Close()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Error("no settings call within 5 s")
	}
}

func addrs(s ...string) []netip.Addr {
	list := make([]netip.Addr, 0, len(s))
	for _, a := range s {
		list = append(list, netip.MustParseAddr(a))
	}
	return list
}

package netmon

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestParse(t *testing.T) {
	t.Parallel()

	ethernet := func(domains ...string) link {
		return link{index: 2, domains: domains, servers: addrs("10.0.2.3"), defaultRoute: true}
	}
	tunnel := func(defaultRoute bool, domains ...string) link {
		return link{index: 5, domains: domains, servers: addrs("10.99.0.1"), tunnel: true, defaultRoute: defaultRoute}
	}
	nameservers := func(s ...string) []nameserver {
		list := make([]nameserver, 0, len(s))
		for _, a := range addrs(s...) {
			list = append(list, nameserver{addr: a})
		}
		return list
	}

	tests := []struct {
		name        string
		sys         system
		wantDomains []string
		wantVPN     bool
	}{
		{name: "router's nameserver", sys: system{nameservers: nameservers("192.168.1.1")}},
		{
			name:        "search lower-cased without its dot",
			sys:         system{search: []string{"Corp.Example.", "other.example"}},
			wantDomains: []string{"corp.example", "other.example"},
		},
		{name: "stub file", sys: system{search: []string{"."}, nameservers: nameservers("127.0.0.53")}},
		{name: "second stub address", sys: system{nameservers: nameservers("127.0.0.54")}},
		{name: "loopback nameserver", sys: system{nameservers: nameservers("127.0.0.1")}, wantVPN: true},
		{name: "IPv6 loopback nameserver", sys: system{nameservers: nameservers("::1")}, wantVPN: true},
		{
			name:    "tunnelled nameserver",
			sys:     system{nameservers: []nameserver{{addr: netip.MustParseAddr("10.99.0.1"), tunnelled: true}}},
			wantVPN: true,
		},
		{
			name: "tunnelled nameserver second",
			sys: system{nameservers: []nameserver{
				{addr: netip.MustParseAddr("192.168.1.1")},
				{addr: netip.MustParseAddr("10.99.0.1"), tunnelled: true},
			}},
			wantVPN: true,
		},
		{
			name:        "ethernet link with domains",
			sys:         system{links: []link{ethernet("corp.example", "vpn.example")}},
			wantDomains: []string{"corp.example", "vpn.example"},
		},
		{
			name: "domain repeated across links and search",
			sys: system{
				links:  []link{ethernet("corp.example"), tunnel(false, "corp.example")},
				search: []string{"corp.example"},
			},
			wantDomains: []string{"corp.example"},
		},
		{name: "tunnel link as default", sys: system{links: []link{tunnel(true)}}, wantVPN: true},
		{name: "tunnel link with the root domain", sys: system{links: []link{tunnel(false, ".")}}, wantVPN: true},
		{
			name:        "tunnel link with a routing domain",
			sys:         system{links: []link{tunnel(false, "wg.example")}},
			wantDomains: []string{"wg.example"},
		},
		{name: "tunnel link as default without servers", sys: system{links: []link{{index: 5, tunnel: true, defaultRoute: true}}}},
		{name: "ethernet link with the root domain", sys: system{links: []link{ethernet(".")}}},
		{
			name: "resolved.conf with the root domain",
			sys:  system{links: []link{{index: 0, domains: []string{"."}, servers: addrs("9.9.9.9")}}},
		},
		{name: "loopback server on link 1", sys: system{links: []link{{index: 1, servers: addrs("127.0.0.1")}}}, wantVPN: true},
		{
			name:    "loopback server after another",
			sys:     system{links: []link{{index: 2, servers: addrs("fe80::1", "::1")}}},
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

func TestParseResolvConf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		data            string
		wantSearch      []string
		wantNameservers []netip.Addr
	}{
		{name: "empty"},
		{
			name: "comments",
			data: "# nameserver 10.0.0.1\n; search hash.example\n#search semi.example\n",
		},
		{
			name:       "domain and search",
			data:       "domain one.example\nsearch two.example three.example\n",
			wantSearch: []string{"one.example", "two.example", "three.example"},
		},
		{
			name:            "tabs",
			data:            "nameserver\t10.0.0.1\nsearch\tcorp.example \t other.example\n",
			wantSearch:      []string{"corp.example", "other.example"},
			wantNameservers: addrs("10.0.0.1"),
		},
		{name: "zone", data: "nameserver fe80::1%enp0s1\n", wantNameservers: addrs("fe80::1%enp0s1")},
		{name: "nameserver not an address", data: "nameserver dns.example\nnameserver 10.0.0.1\n", wantNameservers: addrs("10.0.0.1")},
		{name: "options", data: "options edns0 trust-ad\n"},
		{name: "no newline at the end", data: "nameserver 10.0.0.1", wantNameservers: addrs("10.0.0.1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			search, nameservers := parseResolvConf(tt.data)
			if !slices.Equal(search, tt.wantSearch) || !slices.Equal(nameservers, tt.wantNameservers) {
				t.Errorf("parseResolvConf = %q %v, want %q %v", search, nameservers, tt.wantSearch, tt.wantNameservers)
			}
		})
	}
}

func TestIsTunnel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		flags net.Flags
		want  bool
	}{
		{name: "ethernet", flags: net.FlagUp | net.FlagBroadcast | net.FlagMulticast},
		{name: "loopback", flags: net.FlagUp | net.FlagLoopback},
		{name: "point-to-point", flags: net.FlagUp | net.FlagPointToPoint | net.FlagMulticast, want: true},
		{name: "XFRM device", flags: net.FlagUp, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isTunnel(tt.flags); got != tt.want {
				t.Errorf("isTunnel(%v) = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}

func TestReadNetlinkCallsOnNewSource(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// Each source call takes the next address from sources, so a send returns only once
	// the loop has read the write before it.
	sources := make(chan netip.Addr)
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		readNetlink(r, func() netip.Addr { return <-sources }, func() { calls.Add(1) })
	}()

	first, second := netip.MustParseAddr("10.0.2.15"), netip.MustParseAddr("10.99.0.2")
	sources <- first
	w.Write([]byte{0})
	sources <- first
	w.Write([]byte{0})
	sources <- second
	// Once the loop has ended, every call it made is counted.
	r.Close()
	waitFor(t, done)
	if got := calls.Load(); got != 1 {
		t.Errorf("got %d network calls, want 1", got)
	}
}

func TestWatchResolvConfTargetReplaced(t *testing.T) {
	t.Parallel()
	etc, run := t.TempDir(), t.TempDir()
	path := filepath.Join(etc, "resolv.conf")
	target := filepath.Join(run, "stub-resolv.conf")
	writeFile(t, target)
	symlink(t, target, path)
	called, count := watchCalls(t, path)

	replace(t, target)
	waitFor(t, called)
	if got := count(); got != 1 {
		t.Errorf("got %d settings calls, want 1", got)
	}
}

func TestWatchResolvConfLinkReplaced(t *testing.T) {
	t.Parallel()
	etc, run := t.TempDir(), t.TempDir()
	path := filepath.Join(etc, "resolv.conf")
	target := filepath.Join(run, "stub-resolv.conf")
	writeFile(t, target)
	symlink(t, target, path)
	called, count := watchCalls(t, path)

	symlink(t, filepath.Join(run, "resolv.conf"), path)
	waitFor(t, called)
	if got := count(); got != 1 {
		t.Errorf("got %d settings calls, want 1", got)
	}
}

// TestReadInotifyComparesNames feeds the loop events through a pipe rather than from
// inotify, because the end of the pipe's data tells when it has handled all of them.
func TestReadInotifyComparesNames(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := map[int][]string{1: {"resolv.conf"}, 2: {"stub-resolv.conf"}}
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		readInotify(r, names, func() { calls.Add(1) })
	}()

	events := slices.Concat(
		inotifyEvent(1, "hosts"),
		inotifyEvent(2, "resolv.conf"), // the other watch's name
		inotifyEvent(1, "resolv.conf"),
		inotifyEvent(-1, ""), // IN_Q_OVERFLOW
	)
	if _, err := w.Write(events); err != nil {
		t.Fatal(err)
	}
	w.Close()
	waitFor(t, done)
	if got := calls.Load(); got != 2 {
		t.Errorf("got %d settings calls, want 2", got)
	}
}

func TestWatchResolvConfLinkInSameDirectory(t *testing.T) {
	t.Parallel()
	etc := t.TempDir()
	path := filepath.Join(etc, "resolv.conf")
	target := filepath.Join(etc, "resolv.conf.real")
	writeFile(t, target)
	symlink(t, target, path)
	called, count := watchCalls(t, path)

	replace(t, target)
	waitFor(t, called)
	symlink(t, target, path)
	waitFor(t, called)
	if got := count(); got != 2 {
		t.Errorf("got %d settings calls, want 2", got)
	}
}

func TestWatchResolvConfLinkToMissingTarget(t *testing.T) {
	t.Parallel()
	etc, run := t.TempDir(), t.TempDir()
	path := filepath.Join(etc, "resolv.conf")
	symlink(t, filepath.Join(run, "missing"), path)
	called, count := watchCalls(t, path)

	symlink(t, filepath.Join(run, "other"), path)
	waitFor(t, called)
	if got := count(); got != 1 {
		t.Errorf("got %d settings calls, want 1", got)
	}
}

func TestReadStateWorks(t *testing.T) {
	t.Parallel()
	// A container that copies its host's resolv.conf can have the stub in it and a bus
	// without systemd-resolved.
	if _, err := readState(); err != nil && !errors.Is(err, errResolvedMissing) {
		t.Fatal(err)
	}
}

// TestWatchStopRepeated catches crashes and deadlocks, not a leaked goroutine or a
// callback after stop.
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

func addrs(s ...string) []netip.Addr {
	list := make([]netip.Addr, 0, len(s))
	for _, a := range s {
		list = append(list, netip.MustParseAddr(a))
	}
	return list
}

// watchCalls watches path and returns a channel that gets each settings call, and a
// function that counts them.
func watchCalls(t *testing.T, path string) (called chan struct{}, count func() int32) {
	t.Helper()
	var calls atomic.Int32
	called = make(chan struct{}, 10)
	stop := watchResolvConf(path, func() {
		calls.Add(1)
		called <- struct{}{}
	})
	t.Cleanup(stop)
	return called, calls.Load
}

// inotifyEvent encodes an inotify event for the watch wd, with name padded with zero
// bytes to 16, as the kernel pads it. name must be shorter than that.
func inotifyEvent(wd int32, name string) []byte {
	b, err := binary.Append(nil, binary.NativeEndian, unix.InotifyEvent{Wd: wd, Len: 16})
	if err != nil {
		panic(err)
	}
	padded := make([]byte, 16)
	copy(padded, name)
	return append(b, padded...)
}

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("nothing within 5 s")
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("nameserver 10.0.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// replace renames a new file over path, as NetworkManager and systemd-resolved do.
func replace(t *testing.T, path string) {
	t.Helper()
	tmp := path + ".tmp"
	writeFile(t, tmp)
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

// symlink makes path a symbolic link to target, replacing what is there.
func symlink(t *testing.T, target, path string) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.Symlink(target, tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

package netmon

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The registry keys under HKLM that hold DNS settings.
const (
	// tcpipKey holds the global search list, and under Interfaces each adapter's DNS
	// servers, suffix and search list.
	tcpipKey = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`
	// tcpip6Key holds each adapter's IPv6 DNS servers.
	tcpip6Key = `SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters`
	// dnscacheKey holds the local NRPT rules. A VPN's split-DNS domains are there.
	dnscacheKey = `SYSTEM\CurrentControlSet\Services\Dnscache\Parameters`
	// dnsPolicyKey holds the search list and NRPT rules set by Group Policy.
	dnsPolicyKey = `SOFTWARE\Policies\Microsoft\Windows NT\DNSClient`
	// policiesKey is watched in place of dnsPolicyKey. That key and even its parent can be
	// missing, and a missing key can't be watched.
	policiesKey = `SOFTWARE\Policies`
	// nrptSubkey holds one subkey per NRPT rule, under dnscacheKey and dnsPolicyKey.
	nrptSubkey = `DnsPolicyConfig`
)

// ifTypePropVirtual is the adapter type IF_TYPE_PROP_VIRTUAL, which x/sys doesn't define.
const ifTypePropVirtual = 53

// siteLocal holds fec0:0:0:ffff::1 to ::3, which Windows lists as the DNS servers of an
// adapter that has none. Go's resolver skips them too (net/dnsconfig_windows.go).
var siteLocal = netip.MustParsePrefix("fec0::/10")

// readState fails only when the adapters can't be listed. A missing registry key or value
// is normal and counts as empty, and so does any other error in the settings, which is
// logged.
func readState() (State, error) {
	adapters, err := readAdapters()
	if err != nil {
		return State{}, err
	}
	sys := system{adapters: adapters}
	var errs []error

	sys.primarySuffix, err = primarySuffix()
	errs = append(errs, err)
	for _, path := range []string{tcpipKey, dnsPolicyKey} {
		list, err := searchList(path)
		sys.searchLists = append(sys.searchLists, list)
		errs = append(errs, err)
	}
	for _, path := range []string{dnscacheKey, dnsPolicyKey} {
		rules, err := nrptRules(path + `\` + nrptSubkey)
		sys.rules = append(sys.rules, rules...)
		errs = append(errs, err)
	}

	logSettingsErr(errors.Join(errs...))
	return parse(sys), nil
}

// readAdapters copies what the reader needs from GetAdaptersAddresses.
func readAdapters() ([]adapter, error) {
	// The reader uses neither the adapters' own addresses nor their friendly names.
	const flags = windows.GAA_FLAG_SKIP_UNICAST | windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_FRIENDLY_NAME
	// Microsoft recommends starting with 15 KB. The list can grow between the call that
	// reports the size it needs and the next one, hence the few tries.
	size := uint32(15000)
	var err error
	for range 3 {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err = windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, first, &size)
		if err == nil {
			return copyAdapters(first), nil
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			break
		}
	}
	return nil, fmt.Errorf("list network adapters: %w", err)
}

func copyAdapters(first *windows.IpAdapterAddresses) []adapter {
	var adapters []adapter
	for a := first; a != nil; a = a.Next {
		ad := adapter{
			up:       a.OperStatus == windows.IfOperStatusUp,
			ifType:   a.IfType,
			suffixes: []string{windows.UTF16PtrToString(a.DnsSuffix)},
		}
		for s := a.FirstDnsSuffix; s != nil; s = s.Next {
			ad.suffixes = append(ad.suffixes, windows.UTF16ToString(s.String[:]))
		}
		for d := a.FirstDnsServerAddress; d != nil; d = d.Next {
			if addr, ok := netip.AddrFromSlice(d.Address.IP()); ok {
				ad.servers = append(ad.servers, addr)
			}
		}
		adapters = append(adapters, ad)
	}
	return adapters
}

// primarySuffix returns the primary DNS suffix that is in effect. On a PC joined to a
// domain it is the domain's name.
func primarySuffix() (string, error) {
	// A DNS name has at most 255 characters.
	var buf [256]uint16
	n := uint32(len(buf))
	if err := windows.GetComputerNameEx(windows.ComputerNameDnsDomain, &buf[0], &n); err != nil {
		return "", fmt.Errorf("get primary DNS suffix: %w", err)
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// searchList returns the SearchList value of the key at path.
func searchList(path string) (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return "", settingsErr(err, path)
	}
	defer k.Close()
	list, _, err := k.GetStringValue("SearchList")
	return list, settingsErr(err, path+`\SearchList`)
}

// nrptRules reads the NRPT rules under the key at path, one per subkey.
func nrptRules(path string) ([]nrptRule, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, settingsErr(err, path)
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(0)
	if err != nil {
		return nil, settingsErr(err, path)
	}
	rules := make([]nrptRule, 0, len(names))
	var errs []error
	for _, name := range names {
		r, err := nrptRuleAt(path + `\` + name)
		rules = append(rules, r)
		errs = append(errs, err)
	}
	return rules, errors.Join(errs...)
}

// nrptRuleAt reads the rule in the key at path.
func nrptRuleAt(path string) (nrptRule, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return nrptRule{}, settingsErr(err, path)
	}
	defer k.Close()

	var r nrptRule
	// Name is a multi-string, as Add-DnsClientNrptRule and Tailscale write it.
	r.namespaces, _, err = k.GetStringsValue("Name")
	errs := []error{settingsErr(err, path+`\Name`)}
	for _, value := range []string{"GenericDNSServers", "DirectAccessDNSServers"} {
		servers, _, err := k.GetStringValue(value)
		r.servers = append(r.servers, servers)
		errs = append(errs, settingsErr(err, path+`\`+value))
	}
	return r, errors.Join(errs...)
}

// settingsErr returns nil for an error that means the value is simply empty: a missing
// key or value, or a value of the wrong type. It wraps any other error with path.
func settingsErr(err error, path string) error {
	if err == nil || errors.Is(err, registry.ErrNotExist) || errors.Is(err, registry.ErrUnexpectedType) {
		return nil
	}
	return fmt.Errorf("read %s: %w", path, err)
}

// lastSettingsErr is the text of the last read's joined settings errors, or empty after a
// read without any. It keeps an error that persists from being logged on every read.
// There is one text for all errors, not one per error: with two keys failing with errors
// A and B, each read would log A because it differs from B, then B because it differs
// from A.
var lastSettingsErr struct {
	sync.Mutex
	text string
}

// logSettingsErr logs err, the joined settings errors of one read, unless the last read
// had the same.
func logSettingsErr(err error) {
	text := ""
	if err != nil {
		text = err.Error()
	}
	lastSettingsErr.Lock()
	defer lastSettingsErr.Unlock()
	if text != "" && text != lastSettingsErr.text {
		log.Printf("read DNS settings: %v", err)
	}
	lastSettingsErr.text = text
}

// adapter is what the reader takes from one entry of GetAdaptersAddresses.
type adapter struct {
	up       bool
	ifType   uint32
	suffixes []string     // DnsSuffix and the FirstDnsSuffix list
	servers  []netip.Addr // the DNS servers
}

// nrptRule is one rule of the Name Resolution Policy Table: a key under DnsPolicyConfig.
type nrptRule struct {
	namespaces []string // the Name value
	servers    []string // GenericDNSServers and DirectAccessDNSServers, as stored
}

// system is what the reader takes from Windows.
type system struct {
	adapters      []adapter
	primarySuffix string   // GetComputerNameEx(ComputerNameDnsDomain)
	searchLists   []string // each SearchList value, as stored
	rules         []nrptRule
}

// parse turns what the reader took from Windows into a state.
func parse(sys system) State {
	var s State
	add := func(names ...string) {
		for _, name := range names {
			// A namespace that names a suffix starts with a dot: .vpn.example.
			if name = strings.Trim(strings.ToLower(strings.TrimSpace(name)), "."); name != "" {
				s.Domains = append(s.Domains, name)
			}
		}
	}

	for _, a := range sys.adapters {
		// A disabled adapter keeps its settings in the registry.
		if !a.up {
			continue
		}
		add(a.suffixes...)
		if a.handlesAll() {
			s.VPNHandlesAllDNS = true
		}
	}
	add(sys.primarySuffix)
	for _, list := range sys.searchLists {
		// Windows separates the names with commas.
		add(strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' })...)
	}
	for _, r := range sys.rules {
		// Add-DnsClientNrptRule writes the values it isn't given as empty strings, so a
		// value that exists isn't proof of servers.
		hasServers := slices.ContainsFunc(r.servers, func(v string) bool { return strings.TrimSpace(v) != "" })
		for _, ns := range r.namespaces {
			// The namespace "." means every name. It is checked here, because add would
			// strip it to the empty string, as it does an empty or all-space namespace.
			// A rule without servers only sets options such as DNSSEC.
			if strings.TrimSpace(ns) == "." && hasServers {
				s.VPNHandlesAllDNS = true
			}
			// A namespace counts even if its rule has no servers. That at worst sends a
			// name to the OS that could have gone to the DoH server.
			add(ns)
		}
	}
	return s
}

// handlesAll reports whether a, an adapter that is up, answers every name on behalf of a
// VPN or a DNS tool on this machine.
//
// Windows doesn't choose DNS servers by the default route: each adapter has its own, and
// which one gets a name depends on settings Zen can't see. So a VPN adapter with servers
// counts whether it tunnels everything or not. A VPN whose adapter says it is Ethernet,
// as some company clients' do, is missed.
func (a adapter) handlesAll() bool {
	// PPP for the built-in dial-up VPNs, the proprietary virtual type for Wintun,
	// WireGuardNT and OpenVPN's drivers, and tunnel.
	vpnType := a.ifType == windows.IF_TYPE_PPP || a.ifType == ifTypePropVirtual || a.ifType == windows.IF_TYPE_TUNNEL
	// Every server counts, not the first: Windows lists IPv4 and IPv6 servers together,
	// and a router's IPv6 server can come before a DNS tool's 127.0.0.1.
	for _, server := range a.servers {
		if siteLocal.Contains(server) {
			continue
		}
		// A loopback server is a DNS tool the user or their employer chose, such as
		// dnscrypt-proxy or Cloudflare WARP.
		if vpnType || server.IsLoopback() {
			return true
		}
	}
	return false
}

// watch never fails. What it can't watch it logs and leaves to the monitor's re-reads.
func watch(network, settings func()) (stop func(), err error) {
	w := &watcher{network: network}
	// Windows holds the pointer where the garbage collector can't see it.
	w.pinner.Pin(w)
	notifications := w.watchIPHelper()
	stopKeys := watchKeys([]string{tcpipKey, tcpip6Key, dnscacheKey, policiesKey}, settings)
	return func() {
		// CancelMibChangeNotify2 waits for a callback that is running, so it must not be
		// called from one.
		for _, h := range notifications {
			if err := windows.CancelMibChangeNotify2(h); err != nil {
				log.Printf("stop watching the network: %v", err)
			}
		}
		stopKeys()
		w.pinner.Unpin()
	}, nil
}

// changedCallback is the IP helper's callback for both notifications. It is made once:
// Windows callbacks are never released, and Go ends the program at the 2,001st.
var changedCallback = windows.NewCallback(changed)

// changed runs on a thread of Windows's own, with the watcher as the context.
func changed(w *watcher, _ unsafe.Pointer, _ uint32) uintptr {
	w.network()
	return 0
}

// watcher is what the IP helper's callback gets as its context.
type watcher struct {
	network func()
	pinner  runtime.Pinner
}

// watchIPHelper registers the notifications of interface and address changes, and
// returns their handles.
func (w *watcher) watchIPHelper() []windows.Handle {
	var handles []windows.Handle
	for _, n := range []struct {
		name     string
		register func(uint16, uintptr, unsafe.Pointer, bool, *windows.Handle) error
	}{
		{"interfaces", windows.NotifyIpInterfaceChange},
		{"addresses", windows.NotifyUnicastIpAddressChange},
	} {
		var h windows.Handle
		if err := n.register(windows.AF_UNSPEC, changedCallback, unsafe.Pointer(w), false, &h); err != nil {
			log.Printf("watch %s: %v", n.name, err)
			continue
		}
		handles = append(handles, h)
	}
	return handles
}

// watchKeys watches the registry keys under HKLM at paths, each with its subtree, and
// returns the function that ends the watch.
func watchKeys(paths []string, settings func()) (stop func()) {
	quit, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		log.Printf("watch the registry: %v", err)
		return func() {}
	}
	var keys []watchedKey
	for _, path := range paths {
		k, err := watchKey(path)
		if err != nil {
			log.Printf("watch %s: %v", path, err)
			continue
		}
		keys = append(keys, k)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		waitKeys(quit, keys, settings)
	}()
	return func() {
		windows.SetEvent(quit)
		<-done
		windows.CloseHandle(quit)
	}
}

// waitKeys calls settings after each change of one of keys, until quit is signalled. It
// owns the keys and closes them.
func waitKeys(quit windows.Handle, keys []watchedKey, settings func()) {
	// The keys are closed once no wait is running, because closing a key signals its
	// event. This is a closure because keys shrinks below.
	defer func() {
		for _, k := range keys {
			k.close()
		}
	}()
	for {
		// quit comes first, so that it wins over a key that keeps changing.
		handles := make([]windows.Handle, 0, 1+len(keys))
		handles = append(handles, quit)
		for _, k := range keys {
			handles = append(handles, k.event)
		}
		i, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		if err != nil {
			log.Printf("watch the registry: %v", err)
			return
		}
		if i == windows.WAIT_OBJECT_0 {
			return
		}
		k := keys[i-1]
		// A change between the signal and this call is reported as soon as the key is
		// watched again, so the order of this call and settings doesn't matter.
		if err := k.notify(); err != nil {
			// A key that was deleted can't be watched again, even after it is made again.
			log.Printf("watch %s: %v", k.path, err)
			k.close()
			keys = slices.Delete(keys, int(i-1), int(i))
		}
		settings()
	}
}

// watchedKey is a registry key and the event Windows signals when it changes.
type watchedKey struct {
	path  string
	key   registry.Key
	event windows.Handle
}

func watchKey(path string) (watchedKey, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.NOTIFY)
	if err != nil {
		return watchedKey{}, err
	}
	// Auto-reset, so the wait that reports a change also resets the event.
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		k.Close()
		return watchedKey{}, err
	}
	wk := watchedKey{path: path, key: k, event: event}
	if err := wk.notify(); err != nil {
		wk.close()
		return watchedKey{}, err
	}
	return wk, nil
}

// notify asks Windows to signal the event at the key's next change. Windows signals it
// once per call.
func (k watchedKey) notify() error {
	// Without REG_NOTIFY_THREAD_AGNOSTIC the request belongs to the calling thread, and
	// Windows signals the event when that thread exits. Go ends threads as it likes.
	const filter = windows.REG_NOTIFY_CHANGE_NAME | windows.REG_NOTIFY_CHANGE_LAST_SET | windows.REG_NOTIFY_THREAD_AGNOSTIC
	return windows.RegNotifyChangeKeyValue(windows.Handle(k.key), true, filter, k.event, true)
}

func (k watchedKey) close() {
	k.key.Close()
	windows.CloseHandle(k.event)
}

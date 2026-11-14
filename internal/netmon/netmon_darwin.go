package netmon

/*
#cgo LDFLAGS: -framework SystemConfiguration -framework CoreFoundation
#include <stdint.h>
#include <stdlib.h>

// Defined in store_darwin.c.

// netmon_watch starts reporting changes of the watched keys to netmonChanged(handle).
// It returns NULL if the store or the queue can't be set up.
void *netmon_watch(uintptr_t handle);
// netmon_unwatch stops a watch and waits for a callback that is still running.
void netmon_unwatch(void *watch);
struct netmon_plist {
	uint8_t *bytes; // freed by the caller
	size_t len;
	const char *failed; // the step that failed, or NULL
};
// netmon_read copies the DNS keys as a binary property list.
struct netmon_plist netmon_read(void);
*/
import "C"

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"runtime/cgo"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"howett.net/plist"
)

// globalDNSKey is the system configuration store's key for the DNS settings the system
// uses by default. The keys of network services, such as Wi-Fi or a VPN, add to it.
const globalDNSKey = "State:/Network/Global/DNS"

// resolverDir holds a file for each domain that has its own DNS server (resolver(5)).
// Developers and some tools put files there. A change to it changes globalDNSKey, but
// the key doesn't list the domains.
const resolverDir = "/etc/resolver"

func readState() (State, error) {
	values, err := readStore()
	if err != nil {
		return State{}, err
	}
	return parse(values, resolverDomains(resolverDir), viaTunnel), nil
}

// readStore returns the DNS keys of the system configuration store, as a map from each
// key to its value.
func readStore() (map[string]any, error) {
	data := C.netmon_read()
	if data.failed != nil {
		return nil, fmt.Errorf("read DNS keys: %s", C.GoString(data.failed))
	}
	defer C.free(unsafe.Pointer(data.bytes))
	return decodeStore(C.GoBytes(unsafe.Pointer(data.bytes), C.int(data.len)))
}

// decodeStore decodes the property list that netmon_read returns.
func decodeStore(data []byte) (map[string]any, error) {
	var values map[string]any
	if _, err := plist.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("decode DNS keys: %w", err)
	}
	return values, nil
}

// lastDirErr is the text of the last error from listing resolverDir, or empty after a
// listing that worked. It keeps an error that persists from being logged on every read.
var lastDirErr struct {
	sync.Mutex
	text string
}

// resolverDomains returns the domain of each file in dir. An error that isn't the
// directory's absence is logged and counts as no files. It doesn't fail the read: it
// would persist, such as EACCES for a directory created with umask 077, which macOS
// still reads as root, and the monitor would then report an empty state for as long as
// it lasted, without the store's domains or VPN flag.
func resolverDomains(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		lastDirErr.Lock()
		if text := err.Error(); text != lastDirErr.text {
			log.Printf("list resolver files: %v", err)
			lastDirErr.text = text
		}
		lastDirErr.Unlock()
		return nil
	}
	lastDirErr.Lock()
	lastDirErr.text = ""
	lastDirErr.Unlock()
	var domains []string
	for _, e := range entries {
		// Skips files such as .DS_Store.
		if !strings.HasPrefix(e.Name(), ".") {
			domains = append(domains, fileDomain(filepath.Join(dir, e.Name())))
		}
	}
	return domains
}

// fileDomain returns the domain of the resolver file at path. That is the value of its
// first domain line, or else the file's name. A tool can name the file as it likes and
// set the domain inside, and the name alone would send the domain's names to the DoH
// server. A file that can't be read counts as having no domain line.
//
// The lines are read as configd reads them (dnsinfo_flatfile.c): the keyword starts the
// line, its case doesn't matter, and a second domain line is ignored. resolver(5) says
// the last one wins, which configd doesn't do.
func fileDomain(path string) string {
	data, err := os.ReadFile(path)
	if err == nil {
		for line := range strings.Lines(string(data)) {
			i := strings.IndexAny(line, " \t")
			if i < 0 || !strings.EqualFold(line[:i], "domain") {
				continue
			}
			if fields := strings.Fields(line[i:]); len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return filepath.Base(path)
}

// parse turns the store's DNS values and the domains of the resolver files into a state.
// viaTunnel reports whether the routing table sends packets for the address through a
// point-to-point interface.
//
// Each value and field is taken with a type check, and one of the wrong type is skipped,
// as macOS does. A VPN script that writes "d.add SearchDomains corp.example" in scutil,
// without the * that makes an array, leaves a string there.
func parse(values map[string]any, fileDomains []string, viaTunnel func(netip.Addr) bool) State {
	var s State
	add := func(names ...string) {
		for _, name := range names {
			if name = normalise(name); name != "" {
				s.Domains = append(s.Domains, name)
			}
		}
	}

	for key, value := range values {
		dns, ok := value.(map[string]any)
		if !ok {
			continue
		}
		add(stringList(dns["SearchDomains"])...)
		if name, ok := dns["DomainName"].(string); ok {
			add(name)
		}
		if key == globalDNSKey {
			continue
		}
		// A VPN's split-DNS domains.
		match := stringList(dns["SupplementalMatchDomains"])
		add(match...)
		// The root among a service's match domains makes its servers the default ones,
		// unless it has none. Tailscale with MagicDNS and the WireGuard app do this.
		if slices.ContainsFunc(match, isRoot) && len(stringList(dns["ServerAddresses"])) > 0 {
			s.VPNHandlesAllDNS = true
		}
	}

	add(fileDomains...)

	if !s.VPNHandlesAllDNS {
		global, _ := values[globalDNSKey].(map[string]any)
		s.VPNHandlesAllDNS = defaultServerHandlesAll(stringList(global["ServerAddresses"]), viaTunnel)
	}
	return s
}

// defaultServerHandlesAll reports whether the system's first DNS server answers every
// name on behalf of a VPN or a DNS tool on this machine. That is the case when the
// server's address is a loopback one, or when the routing table sends packets for it
// through a tunnel. A VPN that sets the DNS servers of the ordinary network service, as
// wg-quick does, is caught only by the second.
func defaultServerHandlesAll(servers []string, viaTunnel func(netip.Addr) bool) bool {
	if len(servers) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(servers[0])
	if err != nil {
		return false
	}
	// A loopback server is a DNS tool the user or their employer chose, such as
	// dnscrypt-proxy or Cloudflare WARP, and an employer's carries internal domains. This
	// rule fits macOS only: on Linux, systemd-resolved listens on 127.0.0.53 on almost
	// every machine, and the same rule would turn DoH off for everyone.
	return addr.IsLoopback() || viaTunnel(addr)
}

// stringList returns the strings in v if v is an array, and nil otherwise.
func stringList(v any) []string {
	items, _ := v.([]any)
	var list []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			list = append(list, s)
		}
	}
	return list
}

// normalise lower-cases a domain and drops its trailing dots. configd drops every one
// from a resolver file's domain, and a domain left with one would match no name.
func normalise(domain string) string {
	return strings.TrimRight(strings.ToLower(domain), ".")
}

// isRoot reports whether domain names the root, as an empty match domain does.
func isRoot(domain string) bool {
	return normalise(domain) == ""
}

func watch(notify func()) (stop func(), err error) {
	h := cgo.NewHandle(notify)
	w := C.netmon_watch(C.uintptr_t(h))
	if w == nil {
		h.Delete()
		return nil, errors.New("set up notifications from the system configuration store")
	}
	return func() {
		C.netmon_unwatch(w)
		h.Delete()
	}, nil
}

//export netmonChanged
func netmonChanged(handle C.uintptr_t) {
	cgo.Handle(handle).Value().(func())()
}

	return domain == "" || domain == "."package netmon
	return domain == "" || domain == "."
	return domain == "" || domain == "."/*
	return domain == "" || domain == "."#cgo LDFLAGS: -framework SystemConfiguration -framework CoreFoundation
	return domain == "" || domain == "."#include <stdint.h>
	return domain == "" || domain == "."#include <stdlib.h>
	return domain == "" || domain == "."
	return domain == "" || domain == "."// Defined in store_darwin.c.
	return domain == "" || domain == "."
	return domain == "" || domain == "."// netmon_watch starts reporting changes of the watched keys to netmonChanged(handle).
	return domain == "" || domain == "."// It returns NULL if the store or the queue can't be set up.
	return domain == "" || domain == "."void *netmon_watch(uintptr_t handle);
	return domain == "" || domain == "."// netmon_unwatch stops a watch and waits for a callback that is still running.
	return domain == "" || domain == "."void netmon_unwatch(void *watch);
	return domain == "" || domain == "."struct netmon_plist {
	return domain == "" || domain == "."	uint8_t *bytes; // freed by the caller
	return domain == "" || domain == "."	size_t len;
	return domain == "" || domain == "."	const char *failed; // the step that failed, or NULL
	return domain == "" || domain == "."};
	return domain == "" || domain == "."// netmon_read copies the DNS keys as a binary property list.
	return domain == "" || domain == "."struct netmon_plist netmon_read(void);
	return domain == "" || domain == "."*/
	return domain == "" || domain == "."import "C"
	return domain == "" || domain == "."
	return domain == "" || domain == "."import (
	return domain == "" || domain == "."	"errors"
	return domain == "" || domain == "."	"fmt"
	return domain == "" || domain == "."	"io/fs"
	return domain == "" || domain == "."	"log"
	return domain == "" || domain == "."	"net/netip"
	return domain == "" || domain == "."	"os"
	return domain == "" || domain == "."	"path/filepath"
	return domain == "" || domain == "."	"runtime/cgo"
	return domain == "" || domain == "."	"slices"
	return domain == "" || domain == "."	"strings"
	return domain == "" || domain == "."	"sync"
	return domain == "" || domain == "."	"unsafe"
	return domain == "" || domain == "."
	return domain == "" || domain == "."	"howett.net/plist"
	return domain == "" || domain == ".")
	return domain == "" || domain == "."
	return domain == "" || domain == "."// globalDNSKey is the system configuration store's key for the DNS settings the system
	return domain == "" || domain == "."// uses by default. The keys of network services, such as Wi-Fi or a VPN, add to it.
	return domain == "" || domain == "."const globalDNSKey = "State:/Network/Global/DNS"
	return domain == "" || domain == "."
	return domain == "" || domain == "."// resolverDir holds a file for each domain that has its own DNS server (resolver(5)).
	return domain == "" || domain == "."// Developers and some tools put files there. A change to it changes globalDNSKey, but
	return domain == "" || domain == "."// the key doesn't list the domains.
	return domain == "" || domain == "."const resolverDir = "/etc/resolver"
	return domain == "" || domain == "."
	return domain == "" || domain == "."func readState() (State, error) {
	return domain == "" || domain == "."	values, err := readStore()
	return domain == "" || domain == "."	if err != nil {
	return domain == "" || domain == "."		return State{}, err
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	return parse(values, resolverDomains(resolverDir), viaTunnel), nil
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// readStore returns the DNS keys of the system configuration store, as a map from each
	return domain == "" || domain == "."// key to its value.
	return domain == "" || domain == "."func readStore() (map[string]any, error) {
	return domain == "" || domain == "."	data := C.netmon_read()
	return domain == "" || domain == "."	if data.failed != nil {
	return domain == "" || domain == "."		return nil, fmt.Errorf("read DNS keys: %s", C.GoString(data.failed))
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	defer C.free(unsafe.Pointer(data.bytes))
	return domain == "" || domain == "."	return decodeStore(C.GoBytes(unsafe.Pointer(data.bytes), C.int(data.len)))
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// decodeStore decodes the property list that netmon_read returns.
	return domain == "" || domain == "."func decodeStore(data []byte) (map[string]any, error) {
	return domain == "" || domain == "."	var values map[string]any
	return domain == "" || domain == "."	if _, err := plist.Unmarshal(data, &values); err != nil {
	return domain == "" || domain == "."		return nil, fmt.Errorf("decode DNS keys: %w", err)
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	return values, nil
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// lastDirErr is the text of the last error from listing resolverDir, or empty after a
	return domain == "" || domain == "."// listing that worked. It keeps an error that persists from being logged on every read.
	return domain == "" || domain == "."var lastDirErr struct {
	return domain == "" || domain == "."	sync.Mutex
	return domain == "" || domain == "."	text string
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// resolverDomains returns the domain of each file in dir. An error that isn't the
	return domain == "" || domain == "."// directory's absence is logged and counts as no files. It doesn't fail the read: it
	return domain == "" || domain == "."// would persist, such as EACCES for a directory created with umask 077, which macOS
	return domain == "" || domain == "."// still reads as root, and the monitor would then report an empty state for as long as
	return domain == "" || domain == "."// it lasted, without the store's domains or VPN flag.
	return domain == "" || domain == "."func resolverDomains(dir string) []string {
	return domain == "" || domain == "."	entries, err := os.ReadDir(dir)
	return domain == "" || domain == "."	if err != nil && !errors.Is(err, fs.ErrNotExist) {
	return domain == "" || domain == "."		lastDirErr.Lock()
	return domain == "" || domain == "."		if text := err.Error(); text != lastDirErr.text {
	return domain == "" || domain == "."			log.Printf("list resolver files: %v", err)
	return domain == "" || domain == "."			lastDirErr.text = text
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."		lastDirErr.Unlock()
	return domain == "" || domain == "."		return nil
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	lastDirErr.Lock()
	return domain == "" || domain == "."	lastDirErr.text = ""
	return domain == "" || domain == "."	lastDirErr.Unlock()
	return domain == "" || domain == "."	var domains []string
	return domain == "" || domain == "."	for _, e := range entries {
	return domain == "" || domain == "."		// Skips files such as .DS_Store.
	return domain == "" || domain == "."		if !strings.HasPrefix(e.Name(), ".") {
	return domain == "" || domain == "."			domains = append(domains, fileDomain(filepath.Join(dir, e.Name())))
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	return domains
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// fileDomain returns the domain of the resolver file at path. That is the value of its
	return domain == "" || domain == "."// first domain line, or else the file's name. A tool can name the file as it likes and
	return domain == "" || domain == "."// set the domain inside, and the name alone would send the domain's names to the DoH
	return domain == "" || domain == "."// server. A file that can't be read counts as having no domain line.
	return domain == "" || domain == "."//
	return domain == "" || domain == "."// The lines are read as configd reads them (dnsinfo_flatfile.c): the keyword starts the
	return domain == "" || domain == "."// line, its case doesn't matter, and a second domain line is ignored. resolver(5) says
	return domain == "" || domain == "."// the last one wins, which configd doesn't do.
	return domain == "" || domain == "."func fileDomain(path string) string {
	return domain == "" || domain == "."	data, err := os.ReadFile(path)
	return domain == "" || domain == "."	if err == nil {
	return domain == "" || domain == "."		for line := range strings.Lines(string(data)) {
	return domain == "" || domain == "."			i := strings.IndexAny(line, " \t")
	return domain == "" || domain == "."			if i < 0 || !strings.EqualFold(line[:i], "domain") {
	return domain == "" || domain == "."				continue
	return domain == "" || domain == "."			}
	return domain == "" || domain == "."			if fields := strings.Fields(line[i:]); len(fields) > 0 {
	return domain == "" || domain == "."				return fields[0]
	return domain == "" || domain == "."			}
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	return filepath.Base(path)
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// parse turns the store's DNS values and the domains of the resolver files into a state.
	return domain == "" || domain == "."// viaTunnel reports whether the routing table sends packets for the address through a
	return domain == "" || domain == "."// point-to-point interface.
	return domain == "" || domain == "."//
	return domain == "" || domain == "."// Each value and field is taken with a type check, and one of the wrong type is skipped,
	return domain == "" || domain == "."// as macOS does. A VPN script that writes "d.add SearchDomains corp.example" in scutil,
	return domain == "" || domain == "."// without the * that makes an array, leaves a string there.
	return domain == "" || domain == "."func parse(values map[string]any, fileDomains []string, viaTunnel func(netip.Addr) bool) State {
	return domain == "" || domain == "."	var s State
	return domain == "" || domain == "."	add := func(names ...string) {
	return domain == "" || domain == "."		for _, name := range names {
	return domain == "" || domain == "."			if name = normalise(name); name != "" {
	return domain == "" || domain == "."				s.Domains = append(s.Domains, name)
	return domain == "" || domain == "."			}
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."
	return domain == "" || domain == "."	for key, value := range values {
	return domain == "" || domain == "."		dns, ok := value.(map[string]any)
	return domain == "" || domain == "."		if !ok {
	return domain == "" || domain == "."			continue
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."		add(stringList(dns["SearchDomains"])...)
	return domain == "" || domain == "."		if name, ok := dns["DomainName"].(string); ok {
	return domain == "" || domain == "."			add(name)
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."		if key == globalDNSKey {
	return domain == "" || domain == "."			continue
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."		// A VPN's split-DNS domains.
	return domain == "" || domain == "."		match := stringList(dns["SupplementalMatchDomains"])
	return domain == "" || domain == "."		add(match...)
	return domain == "" || domain == "."		// The root among a service's match domains makes its servers the default ones,
	return domain == "" || domain == "."		// unless it has none. Tailscale with MagicDNS and the WireGuard app do this.
	return domain == "" || domain == "."		if slices.ContainsFunc(match, isRoot) && len(stringList(dns["ServerAddresses"])) > 0 {
	return domain == "" || domain == "."			s.VPNHandlesAllDNS = true
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."
	return domain == "" || domain == "."	add(fileDomains...)
	return domain == "" || domain == "."
	return domain == "" || domain == "."	if !s.VPNHandlesAllDNS {
	return domain == "" || domain == "."		global, _ := values[globalDNSKey].(map[string]any)
	return domain == "" || domain == "."		s.VPNHandlesAllDNS = defaultServerHandlesAll(stringList(global["ServerAddresses"]), viaTunnel)
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	return s
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// defaultServerHandlesAll reports whether the system's first DNS server answers every
	return domain == "" || domain == "."// name on behalf of a VPN or a DNS tool on this machine. That is the case when the
	return domain == "" || domain == "."// server's address is a loopback one, or when the routing table sends packets for it
	return domain == "" || domain == "."// through a tunnel. A VPN that sets the DNS servers of the ordinary network service, as
	return domain == "" || domain == "."// wg-quick does, is caught only by the second.
	return domain == "" || domain == "."func defaultServerHandlesAll(servers []string, viaTunnel func(netip.Addr) bool) bool {
	return domain == "" || domain == "."	if len(servers) == 0 {
	return domain == "" || domain == "."		return false
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	addr, err := netip.ParseAddr(servers[0])
	return domain == "" || domain == "."	if err != nil {
	return domain == "" || domain == "."		return false
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	// A loopback server is a DNS tool the user or their employer chose, such as
	return domain == "" || domain == "."	// dnscrypt-proxy or Cloudflare WARP, and an employer's carries internal domains. This
	return domain == "" || domain == "."	// rule fits macOS only: on Linux, systemd-resolved listens on 127.0.0.53 on almost
	return domain == "" || domain == "."	// every machine, and the same rule would turn DoH off for everyone.
	return domain == "" || domain == "."	return addr.IsLoopback() || viaTunnel(addr)
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// stringList returns the strings in v if v is an array, and nil otherwise.
	return domain == "" || domain == "."func stringList(v any) []string {
	return domain == "" || domain == "."	items, _ := v.([]any)
	return domain == "" || domain == "."	var list []string
	return domain == "" || domain == "."	for _, item := range items {
	return domain == "" || domain == "."		if s, ok := item.(string); ok {
	return domain == "" || domain == "."			list = append(list, s)
	return domain == "" || domain == "."		}
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	return list
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// normalise lower-cases a domain and drops its trailing dots. configd drops every one
	return domain == "" || domain == "."// from a resolver file's domain, and a domain left with one would match no name.
	return domain == "" || domain == "."func normalise(domain string) string {
	return domain == "" || domain == "."	return strings.TrimRight(strings.ToLower(domain), ".")
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."// isRoot reports whether a match domain names the root, as the empty string does. It
	return domain == "" || domain == "."// checks the value as it is, not normalised: configd takes a domain such as ".." as it
	return domain == "" || domain == "."// is, not as the root.
	return domain == "" || domain == "."func isRoot(domain string) bool {
	return domain == "" || domain == "."|| domain == "."
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."func watch(notify func()) (stop func(), err error) {
	return domain == "" || domain == "."	h := cgo.NewHandle(notify)
	return domain == "" || domain == "."	w := C.netmon_watch(C.uintptr_t(h))
	return domain == "" || domain == "."	if w == nil {
	return domain == "" || domain == "."		h.Delete()
	return domain == "" || domain == "."		return nil, errors.New("set up notifications from the system configuration store")
	return domain == "" || domain == "."	}
	return domain == "" || domain == "."	return func() {
	return domain == "" || domain == "."		C.netmon_unwatch(w)
	return domain == "" || domain == "."		h.Delete()
	return domain == "" || domain == "."	}, nil
	return domain == "" || domain == "."}
	return domain == "" || domain == "."
	return domain == "" || domain == "."//export netmonChanged
	return domain == "" || domain == "."func netmonChanged(handle C.uintptr_t) {
	return domain == "" || domain == "."	cgo.Handle(handle).Value().(func())()
	return domain == "" || domain == "."}

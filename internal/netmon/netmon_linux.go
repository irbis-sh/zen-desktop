package netmon

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

const resolvConfPath = "/etc/resolv.conf"

// systemd-resolved's names on the system bus.
const (
	resolvedName = "org.freedesktop.resolve1"
	resolvedPath = dbus.ObjectPath("/org/freedesktop/resolve1")
	managerIface = "org.freedesktop.resolve1.Manager"
	linkIface    = "org.freedesktop.resolve1.Link"
)

// busTimeout is how long a read, or the watch's setup, may wait for the system bus. godbus
// waits for a reply for ever, and a read runs inside StartProxy and StopProxy, so a
// frozen systemd-resolved would otherwise keep the proxy from starting or stopping.
const busTimeout = 2 * time.Second

// errResolvedMissing is the read's error when resolv.conf names systemd-resolved's stub
// but systemd-resolved doesn't answer: it is restarting or broken. Reporting no
// systemd-resolved instead would drop the VPN flag and the domains for that moment.
var errResolvedMissing = errors.New("resolv.conf names systemd-resolved, which isn't answering")

// internetAddr is an address on the internet, used to ask the kernel which address it
// would send from. The resolver's hasIPv4Route uses the same one.
var internetAddr = netip.AddrFrom4([4]byte{8, 8, 8, 8})

// readState reads systemd-resolved over the system bus and /etc/resolv.conf, and takes
// the union of the two. That needs no rule for which of them the machine uses.
func readState() (State, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return State{}, fmt.Errorf("read %s: %w", resolvConfPath, err)
	}
	search, servers := parseResolvConf(string(data))

	// One snapshot serves both the links and the nameservers, so that the two can't
	// disagree about which links are tunnels.
	ifaces, err := net.Interfaces()
	if err != nil {
		return State{}, fmt.Errorf("list network interfaces: %w", err)
	}
	tunnels := make(map[int]net.Interface)
	for _, iface := range ifaces {
		if isTunnel(iface.Flags) {
			tunnels[iface.Index] = iface
		}
	}

	// A failure counts as no systemd-resolved, which is normal on many machines, unless
	// the machine does use it.
	links, err := readResolved(tunnels)
	if err != nil && slices.ContainsFunc(servers, isStub) {
		return State{}, fmt.Errorf("%w: %w", errResolvedMissing, err)
	}

	tunnelled := tunnelAddrs(tunnels)
	nameservers := make([]nameserver, 0, len(servers))
	for _, addr := range servers {
		ns := nameserver{addr: addr}
		// Loopback servers count by themselves, and a dial to one would only find the
		// loopback interface.
		if !addr.IsLoopback() {
			ns.tunnelled = tunnelled[localAddr(addr)]
		}
		nameservers = append(nameservers, ns)
	}
	return parse(system{links: links, search: search, nameservers: nameservers}), nil
}

// readResolved returns what systemd-resolved reports for each link. tunnels holds the
// tunnel links by index. Without a system bus it returns no links and no error.
//
// Every call carries FlagNoAutoStart: without it the bus tries to start systemd-resolved
// on a machine that has it turned off, and writes two lines to the journal each time.
func readResolved(tunnels map[int]net.Interface) ([]link, error) {
	ctx, cancel := context.WithTimeout(context.Background(), busTimeout)
	defer cancel()
	// The context closes the connection when the deadline passes, which ends a call that
	// is waiting. It applies only once the socket is open: godbus connects the socket
	// without a deadline. That blocks only if the bus daemon itself is stuck.
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		// A container, or a sandbox that hides the bus but shows the host's resolv.conf,
		// has the stub in the file and no way to reach systemd-resolved. Failing every
		// read there would throw away the file's search domains.
		return nil, nil
	}
	defer conn.Close()
	manager := conn.Object(resolvedName, resolvedPath)

	var domains []struct {
		Index       int32
		Name        string
		RoutingOnly bool
	}
	if err := getProperty(ctx, manager, managerIface, "Domains", &domains); err != nil {
		return nil, err
	}
	var servers []struct {
		Index  int32
		Family int32
		Addr   []byte
	}
	if err := getProperty(ctx, manager, managerIface, "DNS", &servers); err != nil {
		return nil, err
	}

	var links []link
	at := func(index int32) *link {
		for i := range links {
			if links[i].index == index {
				return &links[i]
			}
		}
		_, tunnel := tunnels[int(index)]
		links = append(links, link{index: index, tunnel: tunnel})
		return &links[len(links)-1]
	}
	for _, d := range domains {
		l := at(d.Index)
		l.domains = append(l.domains, d.Name)
	}
	for _, s := range servers {
		if addr, ok := netip.AddrFromSlice(s.Addr); ok {
			l := at(s.Index)
			l.servers = append(l.servers, addr)
		}
	}

	// Only a tunnel link with servers needs DefaultRoute: no other link's can set the
	// VPN flag.
	for i := range links {
		l := &links[i]
		if !l.tunnel || len(l.servers) == 0 {
			continue
		}
		var path dbus.ObjectPath
		err := manager.CallWithContext(ctx, managerIface+".GetLink", dbus.FlagNoAutoStart, l.index).Store(&path)
		if err == nil {
			err = getProperty(ctx, conn.Object(resolvedName, path), linkIface, "DefaultRoute", &l.defaultRoute)
		}
		// A link's DefaultRoute that can't be read counts as false: the link may have gone
		// since the first calls, and the property is missing before systemd 240. Past the
		// deadline, though, every link would count as false.
		if err != nil && ctx.Err() != nil {
			return nil, err
		}
	}
	return links, nil
}

// getProperty stores the property prop of obj's interface iface in v. godbus's own
// GetProperty doesn't set FlagNoAutoStart.
func getProperty(ctx context.Context, obj dbus.BusObject, iface, prop string, v any) error {
	var variant dbus.Variant
	err := obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", dbus.FlagNoAutoStart, iface, prop).Store(&variant)
	if err == nil {
		err = variant.Store(v)
	}
	if err != nil {
		return fmt.Errorf("get %s.%s: %w", iface, prop, err)
	}
	return nil
}

// tunnelAddrs returns the addresses of the tunnel links.
func tunnelAddrs(tunnels map[int]net.Interface) map[netip.Addr]bool {
	addrs := make(map[netip.Addr]bool)
	for _, iface := range tunnels {
		// Addrs lists every address of the machine on each call, which is why only the
		// tunnel links are asked.
		list, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range list {
			if ipNet, ok := a.(*net.IPNet); ok {
				if addr, ok := netip.AddrFromSlice(ipNet.IP); ok {
					addrs[addr.Unmap()] = true
				}
			}
		}
	}
	return addrs
}

// localAddr returns the address the kernel would send from to reach DNS on remote, or the
// zero address if it has no route. A UDP dial sends nothing, and it applies the kernel's
// routing rules as well as its tables, which a read of the routing table wouldn't.
func localAddr(remote netip.Addr) netip.Addr {
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(remote, 53)))
	if err != nil {
		return netip.Addr{}
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap().WithZone("")
}

// parseResolvConf returns the names of the search and domain lines of a resolv.conf, and
// the address of each nameserver line.
func parseResolvConf(data string) (search []string, nameservers []netip.Addr) {
	for line := range strings.Lines(data) {
		// A line that starts with # or ; has no keyword below as its first field.
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "search", "domain":
			search = append(search, fields[1:]...)
		case "nameserver":
			// ParseAddr accepts a zone, as in fe80::1%enp0s1.
			if addr, err := netip.ParseAddr(fields[1]); err == nil {
				nameservers = append(nameservers, addr)
			}
		}
	}
	return search, nameservers
}

// isTunnel reports whether an interface with flags is a tunnel link: a VPN's or a dial-up
// link. Go's net package reads the flags from the kernel. The point-to-point flag isn't
// enough: the XFRM device that NetworkManager's strongSwan plugin creates for IKEv2 has
// neither it nor broadcast.
func isTunnel(flags net.Flags) bool {
	return flags&(net.FlagBroadcast|net.FlagLoopback) == 0
}

// isStub reports whether addr is one of systemd-resolved's stub listeners.
func isStub(addr netip.Addr) bool {
	return addr == netip.AddrFrom4([4]byte{127, 0, 0, 53}) || addr == netip.AddrFrom4([4]byte{127, 0, 0, 54})
}

// link is what systemd-resolved reports for one network link. Index 0 holds the settings
// from resolved.conf.
type link struct {
	index        int32
	domains      []string     // search and routing domains, "." among them
	servers      []netip.Addr // the DNS servers
	tunnel       bool         // see isTunnel
	defaultRoute bool         // names that no domain claims go to this link
}

// nameserver is one nameserver line of resolv.conf.
type nameserver struct {
	addr      netip.Addr
	tunnelled bool // packets to it leave through a tunnel link
}

// system is what the reader takes from Linux.
type system struct {
	links       []link   // from systemd-resolved, empty without it
	search      []string // the search and domain lines of resolv.conf
	nameservers []nameserver
}

// parse turns what the reader took from Linux into a state.
//
// The VPN flag is set by any of three rules:
//
//  1. A tunnel link has servers and gets the names nobody else claims: it is a default
//     route for DNS, or has the domain ".". systemd-resolved itself asks this for a name
//     that matches no domain. Only tunnel links count, because the main link is a
//     default on every machine. "." in resolved.conf, link 0, is a choice of servers
//     like any other, which the DoH server replaces.
//  2. A nameserver in resolv.conf is tunnelled. Without systemd-resolved, a VPN client
//     writes its server into the file. libc asks the servers in order, so the first one
//     answers almost every name. Any one counts, which errs towards the OS.
//  3. A DNS server is on a loopback address, other than systemd-resolved's stub: a DNS
//     tool such as dnscrypt-proxy or unbound, which may route names by rules Zen can't
//     see.
func parse(sys system) State {
	var s State
	// dnsTool reports whether a DNS server is a DNS tool on this machine.
	dnsTool := func(addr netip.Addr) bool { return addr.IsLoopback() && !isStub(addr) }
	add := func(name string) {
		// The stub file has the line "search .", which this drops.
		if name = strings.TrimSuffix(strings.ToLower(name), "."); name != "" {
			s.Domains = append(s.Domains, name)
		}
	}

	for _, l := range sys.links {
		for _, d := range l.domains {
			add(d)
		}
		// systemd-resolved lists a loopback server under link 1, the loopback interface,
		// whichever link it was set on.
		if slices.ContainsFunc(l.servers, dnsTool) {
			s.VPNHandlesAllDNS = true
		}
		if l.tunnel && len(l.servers) > 0 && (l.defaultRoute || slices.Contains(l.domains, ".")) {
			s.VPNHandlesAllDNS = true
		}
	}
	for _, name := range sys.search {
		add(name)
	}
	for _, ns := range sys.nameservers {
		if ns.tunnelled || dnsTool(ns.addr) {
			s.VPNHandlesAllDNS = true
		}
	}
	return s
}

// watch watches three sources, each with a goroutine of its own: a netlink socket for the
// network, and systemd-resolved's signals and resolv.conf for the settings. It never
// fails: what it can't watch it logs and leaves to the monitor's re-reads.
func watch(network, settings func()) (stop func(), err error) {
	stops := []func(){
		watchNetlink(network),
		watchBus(settings),
		watchResolvConf(resolvConfPath, settings),
	}
	return func() {
		for _, stop := range stops {
			stop()
		}
	}, nil
}

// watchNetlink calls network each time the IPv4 address the machine sends from changes,
// and returns the function that ends the watch.
//
// A netlink message alone isn't a change: the kernel sends an address message at each
// DHCP renewal, and a container's start sends dozens. The source address changes when
// the path to the internet does, including when wg-quick moves traffic into a tunnel with
// a routing rule, which changes no address.
func watchNetlink(network func()) (stop func()) {
	f, err := openNetlink()
	if err != nil {
		log.Printf("watch the network: %v", err)
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		readNetlink(f, func() netip.Addr { return localAddr(internetAddr) }, network)
	}()
	return func() {
		f.Close()
		<-done
	}
}

// openNetlink opens a netlink socket that receives the kernel's IPv4 link, address,
// route and rule messages. It needs no rights.
func openNetlink() (*os.File, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("open netlink socket: %w", err)
	}
	// Without RTMGRP_LINK, a link set down gives no message until it comes up again: the
	// kernel drops its routes without announcing them. Without RTMGRP_IPV4_RULE,
	// wg-quick's rule is missed. IPv6 is left out because the resolver uses DoH only
	// over IPv4, and router advertisements make its messages the noisy ones.
	groups := uint32(unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV4_RULE)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: groups}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("bind netlink socket: %w", err)
	}
	// The socket is non-blocking, so Go's poller handles it, and Close from another
	// goroutine ends a Read that is waiting.
	return os.NewFile(uintptr(fd), "netlink"), nil
}

// readNetlink calls network after a message from f when source returns another address
// than before, until f is closed. The messages aren't parsed.
//
// Two networks that hand out the same address are still told apart, because the address
// goes away in between and source returns the zero address.
func readNetlink(f *os.File, source func() netip.Addr, network func()) {
	last := source()
	// A message longer than buf is cut short, which doesn't matter here.
	buf := make([]byte, 4096)
	for {
		_, err := f.Read(buf)
		// ENOBUFS means the kernel dropped messages because the socket's buffer was full,
		// which is as good a wake-up as any message.
		if err != nil && !errors.Is(err, unix.ENOBUFS) {
			if !errors.Is(err, os.ErrClosed) {
				log.Printf("watch the network: %v", err)
			}
			return
		}
		if src := source(); src != last {
			last = src
			network()
		}
	}
}

// The signals that watchBus asks for.
const (
	propertiesChanged = "org.freedesktop.DBus.Properties.PropertiesChanged"
	nameOwnerChanged  = "org.freedesktop.DBus.NameOwnerChanged"
)

// watchBus calls settings when systemd-resolved or NetworkManager signals a change of
// DNS settings, or systemd-resolved starts or stops. It returns the function that ends the
// watch.
//
// systemd-resolved announces a change of servers and nothing else: not of Domains, nor of
// a link's DefaultRoute. A VPN connecting or disconnecting changes the servers, and
// systemd-resolved rewrites its stub file for a new search domain. NetworkManager
// announces each change of its DNS configuration, which covers a routing-only domain and
// a switch to full DNS when it manages the link. What neither announces, such as a
// routing-only domain set with resolvectl, waits for the monitor's re-read.
func watchBus(settings func()) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	// The connection closes when ctx is done. The deadline covers only the setup.
	deadline := time.AfterFunc(busTimeout, cancel)
	// godbus doesn't drop a signal that doesn't fit: it waits in a goroutine of its own.
	signals := make(chan *dbus.Signal, 16)
	err := subscribe(ctx, signals)
	// Stop returns false once the timer has fired: the connection is closing even if
	// subscribe got through, and ctx.Err() may not show it yet.
	if !deadline.Stop() && err == nil {
		err = context.DeadlineExceeded
	}
	if err != nil {
		cancel()
		log.Printf("watch DNS settings on the system bus: %v", err)
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// godbus closes signals when the connection closes, which ends the loop.
		for sig := range signals {
			// The channel also gets the signals sent to this connection alone, such as
			// NameAcquired right after connecting.
			if sig.Name == propertiesChanged || sig.Name == nameOwnerChanged {
				settings()
			}
		}
		if ctx.Err() == nil {
			log.Printf("watch DNS settings on the system bus: the connection closed")
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// subscribe connects to the system bus and has it send the signals that watchBus wants
// to ch. The connection lasts until ctx is done.
func subscribe(ctx context.Context, ch chan<- *dbus.Signal) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	// The channel comes first, so that no signal arrives before it is there.
	conn.Signal(ch)
	rules := [][]dbus.MatchOption{
		{
			dbus.WithMatchSender(resolvedName),
			dbus.WithMatchObjectPath(resolvedPath),
			dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged"),
		},
		{
			dbus.WithMatchSender("org.freedesktop.DBus"),
			dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, resolvedName),
		},
		// The bus accepts the rule on a machine without NetworkManager.
		{
			dbus.WithMatchSender("org.freedesktop.NetworkManager"),
			dbus.WithMatchObjectPath("/org/freedesktop/NetworkManager/DnsManager"),
			dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged"),
		},
	}
	for _, rule := range rules {
		if err := conn.AddMatchSignalContext(ctx, rule...); err != nil {
			return fmt.Errorf("add match rule: %w", err)
		}
	}
	return nil
}

// resolvConfEvents are the inotify events that watchResolvConf asks for. Most writers
// rename a new file over the old one (IN_MOVED_TO), and a few write it in place
// (IN_CLOSE_WRITE).
const resolvConfEvents = unix.IN_CREATE | unix.IN_MOVED_TO | unix.IN_CLOSE_WRITE | unix.IN_DELETE

// watchResolvConf calls settings after each change of the file at path, and returns the
// function that ends the watch. If path is a symbolic link, the file it resolves to is
// watched too. With systemd-resolved that is the stub file, which it rewrites when the
// search domains change. For a search domain set with resolvectl, this is the only
// notification.
//
// The link is resolved once. If it later points somewhere else, the switch itself is
// a change in path's directory, and changes to the new file wait for the monitor's
// re-reads until the watch starts again.
func watchResolvConf(path string, settings func()) (stop func()) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		log.Printf("watch %s: %v", path, err)
		return func() {}
	}
	paths := []string{path}
	if target, err := filepath.EvalSymlinks(path); err == nil && target != path {
		paths = append(paths, target)
	}
	// names holds the file names to report for each watch. The directories are watched,
	// not the files: a watch on a file would follow the old file after a rename. Both
	// names can be in one directory, which inotify gives one watch.
	names := make(map[int][]string)
	for _, p := range paths {
		wd, err := unix.InotifyAddWatch(fd, filepath.Dir(p), resolvConfEvents)
		if err != nil {
			log.Printf("watch %s: %v", filepath.Dir(p), err)
			continue
		}
		names[wd] = append(names[wd], filepath.Base(p))
	}
	if len(names) == 0 {
		unix.Close(fd)
		return func() {}
	}
	f := os.NewFile(uintptr(fd), "inotify")
	done := make(chan struct{})
	go func() {
		defer close(done)
		readInotify(f, names, settings)
	}()
	return func() {
		f.Close()
		<-done
	}
}

// readInotify calls settings for each event from f whose file name watched lists for the
// event's watch, and when the kernel's event queue overflows, until f is closed.
// Comparing names matters: each call starts the monitor's wait again, so a package
// upgrade that keeps writing to /etc would otherwise hold back a real change for as long
// as it runs.
func readInotify(f *os.File, watched map[int][]string, settings func()) {
	// Room for many events, each a header and a name of at most NAME_MAX bytes.
	buf := make([]byte, 64*(unix.SizeofInotifyEvent+unix.NAME_MAX+1))
	for {
		n, err := f.Read(buf)
		if err != nil {
			if !errors.Is(err, os.ErrClosed) {
				log.Printf("watch resolv.conf: %v", err)
			}
			return
		}
		for off := 0; off < n; {
			var ev unix.InotifyEvent
			size, err := binary.Decode(buf[off:n], binary.NativeEndian, &ev)
			if err != nil || off+size+int(ev.Len) > n {
				break
			}
			// The name is padded with zero bytes.
			name := strings.TrimRight(string(buf[off+size:off+size+int(ev.Len)]), "\x00")
			off += size + int(ev.Len)
			// The kernel dropped events because its queue was full (IN_Q_OVERFLOW).
			if ev.Wd == -1 || slices.Contains(watched[int(ev.Wd)], name) {
				settings()
			}
		}
	}
}

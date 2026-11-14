package netmon

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

// routeTimeout limits a route lookup. The monitor's Close waits for a lookup in progress,
// and the app calls Close with its proxy lock held.
const routeTimeout = time.Second

// routeSeq numbers the lookups, so that each one can find its own reply.
var routeSeq atomic.Int32

// viaTunnel reports whether the routing table sends packets for addr through a
// point-to-point interface. Tunnels are point-to-point (utun, ipsec, ppp, gif), and Wi-Fi
// and Ethernet are not. Any error counts as no.
//
// It asks the routing table rather than dialling. macOS counts a connect to an address on
// the local network as local network access, and the first DNS server is usually the
// router, so a dial would make macOS ask the user to allow Zen to find devices on local
// networks.
func viaTunnel(addr netip.Addr) bool {
	index, err := routeInterface(addr)
	if err != nil {
		return false
	}
	ifi, err := net.InterfaceByIndex(index)
	if err != nil {
		return false
	}
	return ifi.Flags&net.FlagPointToPoint != 0
}

// routeInterface returns the index of the interface that the routing table sends packets
// for addr through.
func routeInterface(addr netip.Addr) (int, error) {
	addr = addr.Unmap()
	var dst route.Addr
	if addr.Is4() {
		dst = &route.Inet4Addr{IP: addr.As4()}
	} else {
		a := &route.Inet6Addr{IP: addr.As16()}
		if zone := addr.Zone(); zone != "" {
			// Without its zone, a link-local address could match the link-local route of
			// any interface, a tunnel's among them.
			ifi, err := net.InterfaceByName(zone)
			if err != nil {
				return 0, fmt.Errorf("find zone %s: %w", zone, err)
			}
			a.ZoneID = ifi.Index
		}
		dst = a
	}

	pid, seq := os.Getpid(), int(routeSeq.Add(1))
	req, err := (&route.RouteMessage{
		Version: unix.RTM_VERSION,
		Type:    unix.RTM_GET,
		Flags:   unix.RTF_UP | unix.RTF_HOST,
		ID:      uintptr(pid),
		Seq:     seq,
		// An empty link address asks for the interface in the reply.
		Addrs: []route.Addr{unix.RTAX_DST: dst, unix.RTAX_IFP: &route.LinkAddr{}},
	}).Marshal()
	if err != nil {
		return 0, fmt.Errorf("encode route request: %w", err)
	}

	fd, err := openRoutingSocket()
	if err != nil {
		return 0, fmt.Errorf("open routing socket: %w", err)
	}
	// Non-blocking, the socket goes through Go's poller, which gives the whole lookup one
	// deadline. A timeout for each read would start again with each message meant for
	// another process, which a routing socket also receives.
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return 0, fmt.Errorf("set routing socket non-blocking: %w", err)
	}
	sock := os.NewFile(uintptr(fd), "route")
	defer sock.Close()
	if err := sock.SetDeadline(time.Now().Add(routeTimeout)); err != nil {
		return 0, fmt.Errorf("set routing socket deadline: %w", err)
	}

	// The kernel answers a failed lookup, such as one with no route, with an error here.
	if _, err := sock.Write(req); err != nil {
		return 0, fmt.Errorf("send route request: %w", err)
	}
	buf := make([]byte, os.Getpagesize())
	for {
		n, err := sock.Read(buf)
		if err != nil {
			return 0, fmt.Errorf("read route reply: %w", err)
		}
		msgs, err := route.ParseRIB(route.RIBTypeRoute, buf[:n])
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			reply, ok := msg.(*route.RouteMessage)
			if !ok || reply.ID != uintptr(pid) || reply.Seq != seq {
				continue
			}
			if reply.Err != nil {
				return 0, fmt.Errorf("look up route: %w", reply.Err)
			}
			if reply.Index == 0 {
				return 0, errors.New("route reply names no interface")
			}
			return reply.Index, nil
		}
	}
}

// openRoutingSocket opens a routing socket that child processes don't inherit. macOS has
// no SOCK_CLOEXEC, so the flag is set after the socket is made, under ForkLock, as the
// standard library does for its own sockets.
func openRoutingSocket() (int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return 0, err
	}
	unix.CloseOnExec(fd)
	return fd, nil
}

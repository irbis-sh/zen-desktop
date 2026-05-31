package process

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

func findPIDBySourcePort(port uint16) (PID, error) {
	if port == 0 {
		return 0, ErrNotFound
	}

	inode, err := findInode(port)
	if err != nil {
		return 0, fmt.Errorf("find inode: %w", err)
	}

	pid, err := findPID(inode)
	if err != nil {
		return 0, fmt.Errorf("find pid: %w", err)
	}

	return pid, nil
}

// findInode uses NETLINK_SOCK_DIAG to find the socket inode
// for a TCP socket with the given local source port (IPv4).
func findInode(port uint16) (uint64, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return 0, fmt.Errorf("netlink socket: %w", err)
	}
	defer unix.Close(fd)

	// Build the request: nlmsghdr + inet_diag_req_v2
	// inet_diag_req_v2: family(1) + protocol(1) + ext(1) + pad(1) + states(4) + id(48) = 56 bytes
	const (
		sizeofNlMsghdr      = 16
		sizeofInetDiagReqV2 = 56
		nlmsgLen            = sizeofNlMsghdr + sizeofInetDiagReqV2
	)

	req := make([]byte, nlmsgLen)

	// nlmsghdr
	binary.LittleEndian.PutUint32(req[0:4], uint32(nlmsgLen))       // nlmsg_len
	binary.LittleEndian.PutUint16(req[4:6], unix.SOCK_DIAG_BY_FAMILY) // nlmsg_type
	binary.LittleEndian.PutUint16(req[6:8], unix.NLM_F_REQUEST|unix.NLM_F_DUMP) // nlmsg_flags
	binary.LittleEndian.PutUint32(req[8:12], 1)                      // nlmsg_seq
	binary.LittleEndian.PutUint32(req[12:16], 0)                     // nlmsg_pid

	// inet_diag_req_v2
	req[16] = unix.AF_INET   // sdiag_family
	req[17] = unix.IPPROTO_TCP // sdiag_protocol
	req[18] = 0               // idiag_ext (no extensions needed)
	req[19] = 0               // pad
	// idiag_states: all states
	binary.LittleEndian.PutUint32(req[20:24], 0xFFFFFFFF)
	// inet_diag_sockid starts at offset 24; all zeros = wildcard (match any addr/port)

	sa := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if err := unix.Sendmsg(fd, req, nil, sa, 0); err != nil {
		return 0, fmt.Errorf("netlink sendmsg: %w", err)
	}

	buf := make([]byte, 4096)
	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			return 0, fmt.Errorf("netlink read: %w", err)
		}

		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return 0, fmt.Errorf("parse netlink message: %w", err)
		}

		for _, msg := range msgs {
			if msg.Header.Type == unix.NLMSG_DONE {
				return 0, ErrNotFound
			}
			if msg.Header.Type == unix.NLMSG_ERROR {
				return 0, fmt.Errorf("netlink error response")
			}

			// inet_diag_msg layout:
			// idiag_family(1) + idiag_state(1) + idiag_timer(1) + idiag_retrans(1)
			// + inet_diag_sockid(48) + idiag_expires(4) + idiag_rqueue(4)
			// + idiag_wqueue(4) + idiag_uid(4) + idiag_inode(4)
			data := msg.Data
			if len(data) < 72 {
				continue
			}

			// inet_diag_sockid.idiag_sport is at offset 4 (within sockid), sockid starts at offset 4
			// So sport is at data[4+4] = data[8], big-endian uint16
			sport := binary.BigEndian.Uint16(data[4:6])
			if sport != port {
				continue
			}

			// idiag_inode is at offset 68 (4 + 48 + 4 + 4 + 4 + 4 = 68), little-endian uint32
			inode := binary.LittleEndian.Uint32(data[68:72])
			if inode == 0 {
				return 0, fmt.Errorf("socket has already been closed")
			}

			return uint64(inode), nil
		}
	}
}

// Silence "imported and not used" for unsafe if compiler complains.

func findPID(inode uint64) (PID, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}

	target := fmt.Sprintf("socket:[%d]", inode)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}

		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			if fd.Type() != fs.ModeSymlink {
				continue
			}

			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}

			if link == target {
				return PID(pid), nil
			}
		}
	}

	return 0, ErrNotFound
}

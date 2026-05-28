package process

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// findInode finds the inode corresponding to a file descriptor
// associated with a TCP socket with the given port.
func findInode(port uint16) (uint64, error) {
	f, err := os.Open("/proc/net/tcp")
	if err != nil {
		return 0, fmt.Errorf("open /proc/net/tcp: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // Skip header line.

	var inode string
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			return 0, fmt.Errorf("parse /proc/net/tcp: expected at least 10 fields, got %d", len(fields))
		}

		localAddr := fields[1]
		_, localPort, found := strings.Cut(localAddr, ":")
		if !found {
			return 0, fmt.Errorf("parse /proc/net/tcp: malformed local addr %q", localAddr)
		}

		localPortNum, err := strconv.ParseUint(localPort, 16, 16)
		if err != nil {
			return 0, fmt.Errorf("parse /proc/net/tcp: parse port %q: %v", localPort, err)
		}

		if uint64(port) == localPortNum {
			inode = fields[9]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read /proc/net/tcp: %v", err)
	}

	if inode == "" {
		return 0, ErrNotFound
	}

	inodeNum, err := strconv.ParseUint(inode, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/net/tcp: parse inode %q: %v", inode, err)
	}
	if inodeNum == 0 {
		return 0, fmt.Errorf("socket has already been closed")
	}

	return inodeNum, nil
}

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
			continue // Not a PID directory.
		}

		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // Permission denied or process gone.
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

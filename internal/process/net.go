package process

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
)

// FindByRequest returns process information for the owner of r's TCP/IPv4
// source port.
//
// Only works for local requests. Returns [ErrNotFound] if no process owns the port.
func FindByRequest(r *http.Request) (Info, error) {
	_, sourcePort, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return Info{}, fmt.Errorf("parse RemoteAddr: %v", err)
	}
	sourcePortNum, err := strconv.ParseUint(sourcePort, 10, 16)
	if err != nil {
		return Info{}, fmt.Errorf("parse source port: %v", err)
	}

	pid, err := findPIDBySourcePort(uint16(sourcePortNum))
	if err != nil {
		return Info{}, err
	}

	info := Info{PID: pid}
	info.ExecutablePath, err = pidExecutablePath(pid)
	if err != nil {
		return info, fmt.Errorf("find executable path for pid %d: %v", pid, err)
	}

	return info, nil
}

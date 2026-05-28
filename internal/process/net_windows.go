package process

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func findPIDBySourcePort(port uint16) (PID, error) {
	if port == 0 {
		return 0, ErrNotFound
	}

	pid, err := findPidByPort(port)
	if err != nil {
		return 0, err
	}
	return PID(pid), nil
}

func findPidByPort(port uint16) (uint32, error) {
	tcpTable, err := getTCPTable()
	if err != nil {
		return 0, fmt.Errorf("get tcp table: %v", err)
	}

	// Pre-convert to network byte order.
	netPort := port<<8 | port>>8

	for _, r := range tcpTable {
		if uint16(r.dwLocalPort) == netPort { // #nosec G115 -- port numbers always fit in uint16
			return r.dwOwningPid, nil
		}
	}
	return 0, ErrNotFound
}

func getTCPTable() ([]mibTcpRowOwnerPid, error) {
	var bufSize uint32
	ret := getExtendedTcpTable(nil, &bufSize, false, windows.AF_INET, tcpTableOwnerPidAll, 0)
	if ret != uint32(windows.ERROR_INSUFFICIENT_BUFFER) {
		return nil, fmt.Errorf("GetExtendedTcpTable size query: %w", syscall.Errno(ret))
	}

	for {
		table := make([]byte, bufSize)
		ret = getExtendedTcpTable(&table[0], &bufSize, false, windows.AF_INET, tcpTableOwnerPidAll, 0)
		switch ret {
		case 0:
			dwNumEntries := int(*(*uint32)(unsafe.Pointer(&table[0])))
			return unsafe.Slice((*mibTcpRowOwnerPid)(unsafe.Pointer(&table[mibTcpTableOwnerPidTableOffset])), dwNumEntries), nil
		case uint32(windows.ERROR_INSUFFICIENT_BUFFER):
			continue
		default:
			return nil, fmt.Errorf("GetExtendedTcpTable: %w", syscall.Errno(ret))
		}
	}
}

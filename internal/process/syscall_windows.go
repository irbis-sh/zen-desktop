package process

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output zsyscall_windows.go syscall_windows.go

//nolint:gocritic
//sys getExtendedTcpTable(pTcpTable *byte, pdwSize *uint32, bOrder bool, ulAf uint32, tableClass uint32, reserved uint32) (ret uint32) = iphlpapi.GetExtendedTcpTable
//sys queryFullProcessImageName(process handle, flags uint32, buffer *uint16, bufferSize *uint32) (err error) = kernel32.QueryFullProcessImageNameW
//sys openProcess(desiredAccess uint32, inheritHandle bool, processId uint32) (process handle, err error) = kernel32.OpenProcess
//sys getFileVersionInfoSize(filename *uint16, handle *uint32) (size uint32, err error) = version.GetFileVersionInfoSizeW
//sys getFileVersionInfo(filename *uint16, handle uint32, len uint32, data *byte) (err error) = version.GetFileVersionInfoW
//sys getLongPathName(shortPath *uint16, longPath *uint16, longPathSize uint32) (requiredSize uint32, err error) = kernel32.GetLongPathNameW
//sys verQueryValue(block *byte, subBlock *uint16, buffer *unsafe.Pointer, len *uint32) (err error) = version.VerQueryValueW

// https://learn.microsoft.com/en-us/windows/win32/api/tcpmib/ns-tcpmib-mib_tcprow_owner_pid
type mibTcpRowOwnerPid struct {
	dwState, dwLocalAddr, dwLocalPort, dwRemoteAddr, dwRemotePort, dwOwningPid uint32
}

type mibTcpTableOwnerPid struct {
	DwNumEntries uint32
	Table        [1]mibTcpRowOwnerPid
}

// https://learn.microsoft.com/en-us/windows/win32/api/winver/nf-winver-verqueryvaluew#varfileinfotranslation
type langAndCodePage struct {
	wLanguage uint16
	wCodePage uint16
}

type handle = windows.Handle

const (
	tcpTableOwnerPidAll            = 5
	mibTcpTableOwnerPidTableOffset = unsafe.Offsetof(mibTcpTableOwnerPid{}.Table)
	// https://learn.microsoft.com/en-us/windows/win32/procthread/process-security-and-access-rights
	processQueryLimitedInformation = 0x1000
)

package process

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func pidExecutablePath(pid PID) (string, error) {
	if pid < 0 || pid > math.MaxUint32 {
		return "", fmt.Errorf("pid out of range for uint32")
	}
	return getProcPath(uint32(pid))
}

func pidName(pid PID, executablePath string) (string, error) {
	path := executablePath
	if path == "" {
		var err error
		path, err = pidExecutablePath(pid)
		if err != nil {
			return "", err
		}
	}

	name, err := getFileDescription(path)
	if err == nil && name != "" {
		return name, nil
	}
	return filepath.Base(path), nil
}

// getFileDescription returns the FileDescription from the executable's version info resource.
func getFileDescription(path string) (string, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	size, err := getFileVersionInfoSize(pathUTF16, nil)
	if err != nil {
		return "", fmt.Errorf("GetFileVersionInfoSize: %w", err)
	}

	data := make([]byte, size)
	if err := getFileVersionInfo(pathUTF16, 0, size, &data[0]); err != nil {
		return "", fmt.Errorf("GetFileVersionInfo: %w", err)
	}

	// Query \VarFileInfo\Translation to get the language/codepage pairs.
	var transPtr unsafe.Pointer
	var transLen uint32
	transQuery, _ := windows.UTF16PtrFromString(`\VarFileInfo\Translation`)
	if err := verQueryValue(&data[0], transQuery, &transPtr, &transLen); err != nil {
		return "", fmt.Errorf("VerQueryValue(Translation): %w", err)
	}
	if transLen < uint32(unsafe.Sizeof(langAndCodePage{})) {
		return "", fmt.Errorf("no translation entries in version info")
	}

	trans := (*langAndCodePage)(transPtr)
	query := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, trans.wLanguage, trans.wCodePage)
	queryUTF16, _ := windows.UTF16PtrFromString(query)

	var descPtr unsafe.Pointer
	var descLen uint32
	if err := verQueryValue(&data[0], queryUTF16, &descPtr, &descLen); err != nil {
		return "", fmt.Errorf("VerQueryValue(FileDescription): %w", err)
	}
	if descLen == 0 {
		return "", nil
	}

	desc := windows.UTF16PtrToString((*uint16)(descPtr))
	return desc, nil
}

func getProcPath(pid uint32) (string, error) {
	proc, err := openProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return "", fmt.Errorf("OpenProcess: %v", err)
	}
	defer windows.CloseHandle(proc)

	bufSize := uint32(256)
	for {
		b := make([]uint16, bufSize)
		err := queryFullProcessImageName(proc, 0, &b[0], &bufSize)
		if errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			bufSize *= 2
			continue
		}
		if err != nil {
			return "", err
		}
		path := windows.UTF16ToString(b[:bufSize])
		return longPathName(path), nil
	}
}

// longPathName resolves 8.3 short names to their long form.
func longPathName(path string) string {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	n, _ := getLongPathName(pathUTF16, nil, 0)
	if n == 0 {
		return path
	}
	buf := make([]uint16, n)
	n, err = getLongPathName(pathUTF16, &buf[0], n)
	if err != nil {
		return path
	}
	return windows.UTF16ToString(buf[:n])
}

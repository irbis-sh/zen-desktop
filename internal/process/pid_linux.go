package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func pidExecutablePath(pid PID) (string, error) {
	exe := fmt.Sprintf("/proc/%d/exe", pid)
	path, err := os.Readlink(exe)
	if err != nil {
		return "", fmt.Errorf("readlink %q: %v", exe, err)
	}
	return path, nil
}

func pidName(pid PID, executablePath string) (string, error) {
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err == nil {
		commStr := strings.TrimRight(string(comm), "\n")
		path := executablePath
		var pathErr error
		if path == "" {
			path, pathErr = pidExecutablePath(pid)
		}
		if pathErr != nil {
			if commStr == "" {
				return "", pathErr
			}
			return commStr, nil
		}

		base := filepath.Base(path)
		if commStr == "" {
			return base, nil
		}
		if len(commStr) > len(base) || commStr != base[:len(commStr)] {
			return commStr, nil
		}
		return base, nil
	}

	if executablePath != "" {
		return filepath.Base(executablePath), nil
	}

	path, err := pidExecutablePath(pid)
	if err != nil {
		return "", err
	}
	return filepath.Base(path), nil
}

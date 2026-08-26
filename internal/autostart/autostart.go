package autostart

import (
	"fmt"
	"os"
)

// Manager manages automatic startup of the app on user login.
type Manager struct{}

// getExecPath returns the path autostart entries should launch.
// ZEN_EXEC_PATH takes precedence over os.Executable because the latter resolves
// wrapper scripts to the real binary: under Nix's wrapGAppsHook3 it reports the
// hidden .zen-wrapped executable, and launching that directly would skip the
// wrapper's environment (certutil on PATH, the tray library, GIO modules).
// The Nix package sets ZEN_EXEC_PATH to the wrapper.
func getExecPath() (string, error) {
	if path := os.Getenv("ZEN_EXEC_PATH"); path != "" {
		return path, nil
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	return execPath, nil
}

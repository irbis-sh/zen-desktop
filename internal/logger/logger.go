package logger

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/irbis-sh/zen-desktop/internal/config"
	"github.com/irbis-sh/zen-desktop/internal/constants"
)

func OpenLogsDirectory() error {
	logsDir, err := getLogsDir(constants.AppName)
	if err != nil {
		return fmt.Errorf("get logs directory: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", logsDir).Start()
	case "darwin":
		return exec.Command("open", logsDir).Start()
	case "linux":
		return exec.Command("xdg-open", logsDir).Start()
	default:
		panic("unsupported platform")
	}
}

func getLogsDir(appName string) (string, error) {
	var path string
	switch runtime.GOOS {
	case "windows":
		path = filepath.Join(os.Getenv("LOCALAPPDATA"), appName, "Logs")
	case "darwin":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home directory: %w", err)
		}
		path = filepath.Join(homeDir, "Library", "Logs", appName)
	case "linux":
		path = filepath.Join(config.DataDir, "logs")
	}

	if err := os.MkdirAll(path, 0755); err != nil { // #nosec G703 -- path is built from system folders and constants
		return "", fmt.Errorf("create log directory: %v", err)
	}

	return path, nil
}

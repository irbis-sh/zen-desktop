// References:
// - https://specifications.freedesktop.org/autostart-spec/autostart-spec-latest.html

package autostart

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"text/template"

	"github.com/irbis-sh/zen-desktop/internal/constants"
)

const (
	desktopTemplate = `[Desktop Entry]
Name={{.Name}}
Comment=Automatically start {{.Name}} at user login
Type=Application
Exec={{.ExecPath}} --start --hidden
Icon=zen-adblocker
X-GNOME-Autostart-enabled=true`
)

type desktopTemplateParameters struct {
	Name     string
	ExecPath string
}

func (m Manager) IsEnabled() (enabled bool, err error) {
	defer func() {
		if err != nil {
			log.Printf("error checking registry key: %s", err)
		}
	}()

	path, err := getDesktopPath()
	if err != nil {
		return false, fmt.Errorf("get desktop path: %w", err)
	}

	_, err = os.Stat(path)
	return err == nil, nil
}

func (m Manager) Enable() (err error) {
	defer func() {
		if err != nil {
			log.Printf("error enabling autostart: %s", err)
		}
	}()

	if enabled, err := m.IsEnabled(); err != nil {
		return fmt.Errorf("check enabled: %w", err)
	} else if enabled {
		return nil
	}

	execPath, err := getExecPath()
	if err != nil {
		return fmt.Errorf("get exec path: %w", err)
	}
	autostartDir, err := getAutostartDir()
	if err != nil {
		return fmt.Errorf("get autostart dir: %w", err)
	}
	desktopPath, err := getDesktopPath()
	if err != nil {
		return fmt.Errorf("get .desktop path: %w", err)
	}

	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return fmt.Errorf("create autostart dir: %w", err)
	}
	f, err := os.Create(desktopPath)
	if err != nil {
		return fmt.Errorf("create .desktop file: %w", err)
	}
	defer f.Close()

	t := template.Must(template.New("desktop").Parse(desktopTemplate))

	if err := t.Execute(f, desktopTemplateParameters{
		Name:     constants.AppName,
		ExecPath: execPath,
	}); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

func (m Manager) Disable() (err error) {
	defer func() {
		if err != nil {
			log.Printf("error disabling autostart: %s", err)
		}
	}()

	if enabled, err := m.IsEnabled(); err != nil {
		return fmt.Errorf("check enabled: %w", err)
	} else if !enabled {
		return nil
	}

	desktopPath, err := getDesktopPath()
	if err != nil {
		return fmt.Errorf("get .desktop path: %w", err)
	}
	if err := os.Remove(desktopPath); err != nil {
		return fmt.Errorf("remove .desktop file: %w", err)
	}

	return nil
}

// getAutostartDir returns the autostart directory as defined in:
// https://specifications.freedesktop.org/autostart-spec/autostart-spec-latest.html
func getAutostartDir() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")

	if configHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home dir: %w", err)
		}
		configHome = filepath.Join(homeDir, ".config")
	}
	
	return filepath.Join(configHome, "autostart"), nil
}

// getDesktopPath returns the path of the .desktop file that autostarts the app.
func getDesktopPath() (string, error) {
	folder, err := getAutostartDir()
	if err != nil {
		return "", fmt.Errorf("get desktop folder: %w", err)
	}

	return filepath.Join(folder, constants.AppName+"-autostart.desktop"), nil
}

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/constants"
)

func TestMigrationV0220RemovesStaleAntiAdblockList(t *testing.T) {
	prevConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() {
		ConfigDir = prevConfigDir
	})

	const removedURL = "https://raw.githubusercontent.com/olegwukr/polish-privacy-filters/master/anti-adblock.txt"
	const keepURL = "https://example.com/keep.txt"

	c := &Config{}
	c.Filter.FilterLists = []FilterList{
		{URL: keepURL},
		{URL: removedURL},
		{URL: removedURL},
	}

	var m *migration
	for i := range migrations {
		if migrations[i].version == "v0.22.0" {
			m = &migrations[i]
			break
		}
	}
	if m == nil {
		t.Fatal("v0.22.0 migration not found")
	}

	if err := m.fn(c); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	if len(c.Filter.FilterLists) != 1 {
		t.Fatalf("expected 1 list after migration, got %d", len(c.Filter.FilterLists))
	}
	if c.Filter.FilterLists[0].URL != keepURL {
		t.Fatalf("unexpected list URL after migration: %s", c.Filter.FilterLists[0].URL)
	}
}

func TestMigrationV0250MovesLinuxAutostartEntry(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("migration is a no-op outside linux")
	}

	m := findMigration(t, "v0.25.0")
	desktopName := constants.AppName + "-autostart.desktop"

	t.Run("moves legacy entry to autostart dir", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		legacyPath := filepath.Join(configHome, desktopName)
		if err := os.WriteFile(legacyPath, []byte("[Desktop Entry]"), 0644); err != nil {
			t.Fatalf("write legacy entry: %v", err)
		}

		if err := m.fn(&Config{}); err != nil {
			t.Fatalf("run migration: %v", err)
		}

		if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
			t.Errorf("legacy entry still present: %v", err)
		}
		newPath := filepath.Join(configHome, "autostart", desktopName)
		data, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("read entry at new location: %v", err)
		}
		if !strings.Contains(string(data), "Exec=") {
			t.Errorf("entry at new location lacks an Exec line: %q", data)
		}
	})

	t.Run("keeps existing entry at new location", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		legacyPath := filepath.Join(configHome, desktopName)
		if err := os.WriteFile(legacyPath, []byte("[Desktop Entry]"), 0644); err != nil {
			t.Fatalf("write legacy entry: %v", err)
		}
		newPath := filepath.Join(configHome, "autostart", desktopName)
		if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
			t.Fatalf("create autostart dir: %v", err)
		}
		const existing = "[Desktop Entry]\nExec=/opt/zen --start --hidden"
		if err := os.WriteFile(newPath, []byte(existing), 0644); err != nil {
			t.Fatalf("write entry at new location: %v", err)
		}

		if err := m.fn(&Config{}); err != nil {
			t.Fatalf("run migration: %v", err)
		}

		if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
			t.Errorf("legacy entry still present: %v", err)
		}
		data, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("read entry at new location: %v", err)
		}
		if string(data) != existing {
			t.Errorf("entry at new location was modified: %q", data)
		}
	})

	t.Run("removes legacy entry even if enable fails", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		legacyPath := filepath.Join(configHome, desktopName)
		if err := os.WriteFile(legacyPath, []byte("[Desktop Entry]"), 0644); err != nil {
			t.Fatalf("write legacy entry: %v", err)
		}
		// A regular file at the autostart dir path makes Enable's MkdirAll fail.
		if err := os.WriteFile(filepath.Join(configHome, "autostart"), nil, 0644); err != nil {
			t.Fatalf("write autostart blocker file: %v", err)
		}

		if err := m.fn(&Config{}); err == nil {
			t.Fatal("expected migration to return an error")
		}

		if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
			t.Errorf("legacy entry still present: %v", err)
		}
	})

	t.Run("no-op without legacy entry", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		if err := m.fn(&Config{}); err != nil {
			t.Fatalf("run migration: %v", err)
		}

		newPath := filepath.Join(configHome, "autostart", desktopName)
		if _, err := os.Stat(newPath); !os.IsNotExist(err) {
			t.Errorf("entry unexpectedly created at new location: %v", err)
		}
	})
}

func findMigration(t *testing.T, version string) *migration {
	t.Helper()
	for i := range migrations {
		if migrations[i].version == version {
			return &migrations[i]
		}
	}
	t.Fatalf("%s migration not found", version)
	return nil
}

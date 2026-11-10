package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRunMigrations(t *testing.T) {
	tests := []struct {
		name        string
		noFile      bool
		applied     string
		wantRan     []int
		wantApplied string
	}{
		{"missing file runs all", true, "", []int{0, 1, 2}, "3"},
		{"runs only pending", false, "1", []int{1, 2}, "3"},
		{"up to date runs nothing", false, "3", nil, "3"},
		{"downgrade keeps higher count", false, "5", nil, "5"},
		{"trailing newline is accepted", false, "1\n", []int{1, 2}, "3"},
		{"unparsable file counts all as applied", false, "junk", nil, "3"},
		{"empty file counts all as applied", false, "", nil, "3"},
		{"negative count counts all as applied", false, "-1", nil, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prevConfigDir, prevMigrations := ConfigDir, migrations
			t.Cleanup(func() {
				ConfigDir, migrations = prevConfigDir, prevMigrations
			})
			ConfigDir = t.TempDir()
			var ran []int
			migrations = make([]migration, 0, 3)
			for i := range 3 {
				migrations = append(migrations, migration{"test", func(*Config) error {
					ran = append(ran, i)
					return nil
				}})
			}

			appliedFile := filepath.Join(ConfigDir, "applied_migrations")
			if !tt.noFile {
				if err := os.WriteFile(appliedFile, []byte(tt.applied), 0644); err != nil {
					t.Fatalf("write applied migrations file: %v", err)
				}
			}

			c := &Config{}
			c.runMigrations()

			if !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran migrations %v, want %v", ran, tt.wantRan)
			}
			data, err := os.ReadFile(appliedFile)
			if err != nil {
				t.Fatalf("read applied migrations file: %v", err)
			}
			if string(data) != tt.wantApplied {
				t.Errorf("applied migrations file is %q, want %q", data, tt.wantApplied)
			}
		})
	}
}

// TestFirstLaunchConfigSkipsMigrations checks that a process exiting between
// New and RunMigrations, as zen --uninstall-ca does, doesn't make the next
// launch run the migrations over the default config.
func TestFirstLaunchConfigSkipsMigrations(t *testing.T) {
	prevConfigDir, prevLegacyMigrations, prevMigrations := ConfigDir, legacyMigrations, migrations
	t.Cleanup(func() {
		ConfigDir, legacyMigrations, migrations = prevConfigDir, prevLegacyMigrations, prevMigrations
	})
	ConfigDir = t.TempDir()
	// Stubs rather than the real lists: some legacy migrations change state
	// outside ConfigDir, such as the autostart entry.
	var ran []string
	legacyMigrations = []legacyMigration{{"v0.28.0", func(*Config) error {
		ran = append(ran, "legacy v0.28.0")
		return nil
	}}}
	migrations = make([]migration, 0, 2)
	for i := range 2 {
		migrations = append(migrations, migration{"test", func(*Config) error {
			ran = append(ran, fmt.Sprintf("migration %d", i))
			return nil
		}})
	}

	if _, err := New(); err != nil {
		t.Fatalf("New on first launch: %v", err)
	}
	// Builds before v0.29.0 only read last_migration.
	data, err := os.ReadFile(filepath.Join(ConfigDir, "last_migration"))
	if err != nil {
		t.Fatalf("read last migration file: %v", err)
	}
	if string(data) != "v0.28.0" {
		t.Errorf("last migration file is %q, want %q", data, "v0.28.0")
	}

	c, err := New()
	if err != nil {
		t.Fatalf("New on next launch: %v", err)
	}
	c.runLegacyMigrations()
	c.runMigrations()

	if len(ran) > 0 {
		t.Errorf("next launch ran %v, want none", ran)
	}
}

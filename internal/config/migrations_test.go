package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRunMigrations(t *testing.T) {
	tests := []struct {
		name        string
		firstLaunch bool
		noFile      bool
		applied     string
		wantRan     []int
		wantApplied string
	}{
		{"first launch runs nothing", true, true, "", nil, "3"},
		{"missing file runs all", false, true, "", []int{0, 1, 2}, "3"},
		{"runs only pending", false, false, "1", []int{1, 2}, "3"},
		{"up to date runs nothing", false, false, "3", nil, "3"},
		{"downgrade keeps higher count", false, false, "5", nil, "5"},
		{"trailing newline is accepted", false, false, "1\n", []int{1, 2}, "3"},
		{"unparsable file counts all as applied", false, false, "junk", nil, "3"},
		{"empty file counts all as applied", false, false, "", nil, "3"},
		{"negative count counts all as applied", false, false, "-1", nil, "3"},
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

			c := &Config{firstLaunch: tt.firstLaunch}
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

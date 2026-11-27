package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type migration struct {
	// name describes the migration in logs.
	name string
	fn   func(c *Config) error
}

// migrations is an ordered list of migrations. The applied_migrations file
// records how many of them have run, so an entry is identified by its position
// alone, not by the version of the build that runs it.
//
// Maintainers:
//   - Always append new migrations to the end of this slice. Never reorder or
//     remove entries.
//   - Never change a migration once it is merged. Builds from master may have
//     run it already, so ship a fix as a new migration.
//   - Make the same change to default-config.json. A fresh install records
//     every migration as applied without running any.
var migrations = []migration{
	{"remove legacy Linux folders", func(*Config) error {
		if runtime.GOOS != "linux" {
			return nil
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get user home dir: %w", err)
		}
		dataHome := filepath.Dir(DataDir)
		dirs := []string{
			// The logs folder, which ignored $XDG_DATA_HOME.
			filepath.Join(homeDir, ".local", "share", "Zen"),
			// Until prgname_linux.go fixed GLib's program name, the webview named its
			// folders after the binary: Zen in the release archive, zen-adblocker in
			// the AUR package.
			filepath.Join(dataHome, "Zen"),
			filepath.Join(dataHome, "zen-adblocker"),
		}
		// On a case-insensitive filesystem, a Zen folder is the zen folder next to
		// it. These hold Zen's config, certificates and cache, and Zen Browser uses
		// ~/.cache/zen as well.
		keep := []string{ConfigDir, DataDir}
		if cacheHome, err := os.UserCacheDir(); err == nil {
			dirs = append(dirs, filepath.Join(cacheHome, "Zen"), filepath.Join(cacheHome, "zen-adblocker"))
			keep = append(keep, filepath.Join(cacheHome, "zen"))
		}

		return removeLegacyFolders(dirs, keep)
	}},
	{"v0.28.0", func(c *Config) error {
		if err := c.AddFilterList(FilterList{
			Name:    "Fanboy's Annoyance List",
			Type:    "annoyances",
			URL:     "https://secure.fanboy.co.nz/fanboy-annoyance.txt",
			Enabled: false,
		}); err != nil {
			return err
		}
		return nil
	}},
}

// RunMigrations brings the config, and any other state migrations touch, up to
// date with this build.
func (c *Config) RunMigrations() {
	if Version == "development" {
		log.Println("skipping migrations in development mode")
		return
	}

	c.runLegacyMigrations()
	c.runMigrations()
}

const appliedMigrationsFile = "applied_migrations"

// runMigrations runs the migrations past the count in the applied_migrations
// file, then records the new count.
func (c *Config) runMigrations() {
	var applied int
	// recorded is the file's content, so that an unchanged count isn't rewritten.
	// Each write truncates the file first, and a crash at that moment empties it.
	var recorded string
	data, err := os.ReadFile(filepath.Join(ConfigDir, appliedMigrationsFile))
	switch {
	case os.IsNotExist(err):
		// Updating from a build before the file existed.
	case err != nil:
		// Possibly temporary, such as wrong ownership after running as root.
		// Leave the file alone and try again on the next launch.
		log.Printf("error reading applied migrations file: %v", err)
		return
	default:
		recorded = string(data)
		applied, err = strconv.Atoi(strings.TrimSpace(recorded))
		if err != nil || applied < 0 {
			// There is no telling which migrations have run. Running them
			// again could undo the user's settings, so count them all as
			// applied: the pending ones are lost, but later ones still run.
			log.Printf("invalid applied migrations file %q, counting all migrations as applied", recorded)
			applied = len(migrations)
		}
	}

	for i := applied; i < len(migrations); i++ {
		m := migrations[i]
		if err := m.fn(c); err != nil {
			log.Printf("error running migration %d (%s): %v", i, m.name, err)
		} else {
			log.Printf("ran migration %d (%s)", i, m.name)
		}
	}

	// After a downgrade, the file counts migrations this build doesn't have.
	// Keep that count, or they would run again after the next upgrade.
	count := max(applied, len(migrations))
	if strconv.Itoa(count) == recorded {
		return
	}
	if err := writeAppliedMigrations(count); err != nil {
		log.Printf("error writing applied migrations file: %v", err)
	}
}

func writeAppliedMigrations(count int) error {
	return os.WriteFile(filepath.Join(ConfigDir, appliedMigrationsFile), []byte(strconv.Itoa(count)), 0644)
}

// removeLegacyFolders deletes dirs, skipping any that is the same folder as one
// in keep.
func removeLegacyFolders(dirs, keep []string) error {
	var errs []error
outer:
	for _, dir := range dirs {
		for _, k := range keep {
			if sameDir(dir, k) {
				continue outer
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", dir, err))
		}
	}
	return errors.Join(errs...)
}

func sameDir(a, b string) bool {
	aInfo, err := os.Stat(a)
	if err != nil {
		return false
	}
	bInfo, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(aInfo, bInfo)
}

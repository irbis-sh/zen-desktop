package config

import (
	"log"
	"os"
	"path/filepath"
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
var migrations = []migration{}

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
	appliedFile := filepath.Join(ConfigDir, appliedMigrationsFile)

	var applied int
	// recorded is the file's content, so that an unchanged count isn't rewritten.
	// Each write truncates the file first, and a crash at that moment empties it.
	var recorded string
	if c.firstLaunch {
		// The default config already reflects every migration in this build.
		applied = len(migrations)
	} else {
		data, err := os.ReadFile(appliedFile)
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
	count := strconv.Itoa(max(applied, len(migrations)))
	if count == recorded {
		return
	}
	if err := os.WriteFile(appliedFile, []byte(count), 0644); err != nil {
		log.Printf("error writing applied migrations file: %v", err)
	}
}

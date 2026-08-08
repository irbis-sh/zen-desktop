package localcdn

import (
	"sync"

	"github.com/irbis-sh/zen-desktop/internal/config"
)

// Stats tracks local resource injection counters. Safe for concurrent use.
type Stats struct {
	mu sync.Mutex

	totalSinceInstall int64
	totalSinceReset   int64
	totalFilterHits   int64
	byLibrary         map[string]int64
	byCDN             map[string]int64
}

// newStats creates a stats counter seeded from a persisted snapshot.
func newStats(initial config.LocalResourcesStats) *Stats {
	byLibrary := initial.ByLibrary
	if byLibrary == nil {
		byLibrary = make(map[string]int64)
	}
	byCDN := initial.ByCDN
	if byCDN == nil {
		byCDN = make(map[string]int64)
	}
	return &Stats{
		totalSinceInstall: initial.TotalSinceInstall,
		totalSinceReset:   initial.TotalSinceReset,
		totalFilterHits:   initial.FilterHits,
		byLibrary:         byLibrary,
		byCDN:             byCDN,
	}
}

// record counts one locally served resource.
func (s *Stats) record(library, cdnHost string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalSinceInstall++
	s.totalSinceReset++
	s.byLibrary[library]++
	s.byCDN[cdnHost]++
}

// RecordFilterHit counts one request handled by an ad-blocking filter list.
func (s *Stats) RecordFilterHit() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalFilterHits++
}

// Snapshot returns a copy of the current counters.
func (s *Stats) Snapshot() config.LocalResourcesStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return config.LocalResourcesStats{
		TotalSinceInstall: s.totalSinceInstall,
		TotalSinceReset:   s.totalSinceReset,
		FilterHits:        s.totalFilterHits,
		ByLibrary:         cloneCounts(s.byLibrary),
		ByCDN:             cloneCounts(s.byCDN),
	}
}

// Reset zeroes the since-reset counters.
func (s *Stats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalSinceReset = 0
	s.totalFilterHits = 0
	s.byLibrary = make(map[string]int64)
	s.byCDN = make(map[string]int64)
}

// cloneCounts copies a counters map.
func cloneCounts(m map[string]int64) map[string]int64 {
	clone := make(map[string]int64, len(m))
	for key, value := range m {
		clone[key] = value
	}
	return clone
}

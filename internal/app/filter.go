package app

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/irbis-sh/zen-desktop/internal/filter"
	"github.com/irbis-sh/zen-desktop/internal/filterliststore"
)

const myRulesFilterName = "My rules"

// initFilter populates f with every enabled filter list plus the user's own
// rules, and reports the merged outcome across all lists. It returns after
// every list has been fetched and parsed.
func (a *App) initFilter(ctx context.Context, f *filter.Filter, mode filterliststore.FetchMode) filter.Outcome {
	var outcome filter.Outcome
	var outcomeMu sync.Mutex

	var wg sync.WaitGroup
	for _, filterList := range a.config.GetFilterLists() {
		if !filterList.Enabled {
			continue
		}
		wg.Go(func() {
			res := f.AddURL(ctx, filterList.URL, filterList.Name, filterList.Trusted, mode)
			if res.Err != nil {
				// The flags matter: a truncated or partially failed list still
				// contributed most of its rules, unlike one that failed outright.
				log.Printf("filter list %q: truncated=%v failed=%v stale=%v: %v",
					filterList.URL, res.Truncated, res.Failed, res.ServedStale, res.Err)
			}
			outcomeMu.Lock()
			outcome = outcome.Merge(res)
			outcomeMu.Unlock()
		})
	}

	wg.Go(func() {
		myRules := a.config.GetRules()
		reader := strings.NewReader(strings.Join(myRules, "\n"))
		if err := f.AddReader(reader, myRulesFilterName, true); err != nil {
			log.Printf("failed to add my rules to filter: %v", err)
		}
	})

	wg.Wait()

	f.Finalize()

	return outcome
}

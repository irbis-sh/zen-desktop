package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/asset"
	"github.com/irbis-sh/zen-desktop/internal/filter"
	"github.com/irbis-sh/zen-desktop/internal/filter/whitelistserver"
	"github.com/irbis-sh/zen-desktop/internal/filterliststore"
	"github.com/irbis-sh/zen-desktop/internal/networkrules"
)

const myRulesFilterName = "My rules"

// filterBuildTimeout bounds the whole build loop. StartProxy holds proxyMu
// for its duration, so quit and StopProxy block until the loop returns; on a
// blackholed network the per-fetch budgets alone could stack up to minutes.
const filterBuildTimeout = 90 * time.Second

// passModes ladders successive build passes away from the network: a pass
// that saw a truncated stream retries under the next mode, and the final
// pass reads only from cache, where mid-parse breaks are all but impossible
// (cache serves are read into memory up front).
var passModes = []filterliststore.FetchMode{
	filterliststore.ModeDefault,
	filterliststore.ModePreferCache,
	filterliststore.ModeCacheOnly,
}

// buildFilter constructs a fully populated, finalized filter together with
// the whitelist server and asset engine wired into it. The caller must start
// and serve exactly these instances: allowlisting inserts rules through the
// whitelist server into this pass's rule store, and the asset server must
// serve this pass's engine.
//
// A stream that breaks mid-body leaves partially parsed rules behind -
// including a possible trailing fragment applied as a rule - so a truncated
// pass is not patched: its whole structure is discarded and rebuilt in the
// next pass, mostly from the copies the previous pass promoted to cache.
// Failed lists don't trigger a rebuild: refetching cannot help a list with
// no network and no cache, so they are skipped (initFilter logs each).
func (a *App) buildFilter() (*filter.Filter, *whitelistserver.Server, *asset.Engine, error) {
	ctx, cancel := context.WithTimeout(context.Background(), filterBuildTimeout)
	defer cancel()

	var (
		f             *filter.Filter
		whitelistSrv  *whitelistserver.Server
		assetInjector *asset.Engine
	)
	for pass, mode := range passModes {
		if ctx.Err() != nil {
			// The deadline is spent: degrade straight to cache-only, which
			// never touches the network or consults the context.
			mode = filterliststore.ModeCacheOnly
		}

		// Reassigning drops the previous pass's tainted structures before
		// the heavy parsing starts, so peak memory stays bounded by one full
		// rule tree plus the one being built.
		networkRules := networkrules.New()
		whitelistSrv = whitelistserver.New(networkRules)
		var err error
		assetInjector, err = asset.NewEngine(a.config.GetAssetPort())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("create asset injector: %v", err)
		}
		f, err = filter.NewFilter(networkRules, assetInjector, a.filterListStore, a.frontendEvents, whitelistSrv)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("create filter: %v", err)
		}

		outcome := a.initFilter(ctx, f, mode)
		if !outcome.Truncated {
			break
		}
		if pass == len(passModes)-1 {
			// Theoretically reachable (a disk read can break too), but the
			// hard pass cap wins over completeness: serve what parsed.
			log.Printf("filter lists still truncated after %d passes, continuing with incomplete rules", len(passModes))
			break
		}
		log.Printf("truncated filter list detected on pass %d, rebuilding", pass+1)
	}

	f.Finalize()
	return f, whitelistSrv, assetInjector, nil
}

// initFilter populates f with every enabled filter list plus the user's own
// rules, and reports the merged outcome across all lists. It returns after
// every list has been fetched and parsed. f is left unfinalized: compacting
// a structure that a truncated outcome is about to discard would be wasted
// work, so buildFilter finalizes the accepted pass only.
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

	return outcome
}

package app

import (
	"context"
	"errors"
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

// filterBuildTimeout bounds the network phase of the build: once it expires,
// remaining passes are forced to cache-only. It is not a hard ceiling on the
// loop - cache-only work never consults the context and a pass that is
// already parsing runs to quiescence - so the build can overrun it by parse
// CPU (seconds), never by network waits, which on a blackholed network could
// otherwise stack up to minutes. It is not what bounds a stop: StopProxy
// aborts the build instead of waiting the deadline out (see errBuildAborted).
const filterBuildTimeout = 90 * time.Second

// errBuildAborted is the cancellation cause StopProxy sets to abandon an
// in-flight build. It means "the user asked to stop, unwind" and makes
// runBuildPasses return, unlike deadline expiry, which means "degrade to
// cache-only and finish".
var errBuildAborted = errors.New("build aborted")

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
func (a *App) buildFilter() (*filter.Filter, *whitelistserver.Server, *asset.Engine, error) {
	// The abort signal lives on a parent of the timeout context: a context
	// only records its first cancellation cause, so an abort arriving after
	// the deadline has fired would be invisible on a shared one. An abort
	// still cancels ctx through parent-child propagation, draining fetches
	// exactly like deadline expiry does.
	abortCtx, abort := context.WithCancelCause(context.Background())
	defer abort(nil)
	ctx, cancelTimeout := context.WithTimeout(abortCtx, filterBuildTimeout)
	defer cancelTimeout()
	a.setBuildAbort(abort)
	defer a.setBuildAbort(nil)
	aborted := func() bool { return errors.Is(context.Cause(abortCtx), errBuildAborted) }

	var (
		f             *filter.Filter
		whitelistSrv  *whitelistserver.Server
		assetInjector *asset.Engine
	)
	err := runBuildPasses(ctx, aborted, func(ctx context.Context, mode filterliststore.FetchMode) (filter.Outcome, error) {
		// Reassigning drops the previous pass's tainted structures before
		// the heavy parsing starts, so peak memory stays bounded by one full
		// rule tree plus the one being built.
		networkRules := networkrules.New()
		whitelistSrv = whitelistserver.New(networkRules)
		var err error
		assetInjector, err = asset.NewEngine(a.config.GetAssetPort())
		if err != nil {
			return filter.Outcome{}, fmt.Errorf("create asset injector: %v", err)
		}
		f, err = filter.NewFilter(networkRules, assetInjector, a.filterListStore, a.frontendEvents, whitelistSrv)
		if err != nil {
			return filter.Outcome{}, fmt.Errorf("create filter: %v", err)
		}
		return a.populateFilter(ctx, f, mode), nil
	})
	if err != nil {
		return nil, nil, nil, err
	}

	f.Finalize()
	return f, whitelistSrv, assetInjector, nil
}

// runBuildPasses drives buildPass down the passModes ladder until a pass is
// accepted or the cap is hit, and stops early on a construction error or an
// abort.
//
// A stream that breaks mid-body leaves partially parsed rules behind -
// including a possible trailing fragment applied as a rule - so a truncated
// pass is not patched: its whole structure is discarded and rebuilt in the
// next pass, mostly from the copies the previous pass promoted to cache.
// Failed lists don't trigger a rebuild: refetching cannot help a list with
// no network and no cache, so they are skipped (populateFilter logs each). The
// exception is lists the build deadline cut off - they fail before their
// cached copies are consulted, so one extra cache-only pass recovers them.
func runBuildPasses(ctx context.Context, aborted func() bool, buildPass func(ctx context.Context, mode filterliststore.FetchMode) (filter.Outcome, error)) error {
	for pass, mode := range passModes {
		if aborted() {
			return errBuildAborted
		}
		if ctx.Err() != nil {
			// The deadline is spent: degrade straight to cache-only, which
			// never touches the network or consults the context.
			mode = filterliststore.ModeCacheOnly
		}

		outcome, err := buildPass(ctx, mode)
		// Checked before err on purpose: a construction failure that
		// coincides with an abort is still a deliberate stop, not a start
		// error. And a mid-pass abort must not start a rebuild pass or
		// accept this one either: StopProxy is already waiting on proxyMu
		// and would immediately tear down whatever StartProxy went on to
		// start.
		if aborted() {
			return errBuildAborted
		}
		if err != nil {
			return err
		}
		// A Failed list normally has neither network nor cache. When the
		// deadline expired mid-pass, however, Failed can also mean the list
		// was cut off before reaching its cached copy: Get refuses to dress
		// caller cancellation up as a stale serve, so the recovery has to
		// happen here, in the pass forced to cache-only above.
		deadlineCutOff := outcome.Failed && ctx.Err() != nil && mode != filterliststore.ModeCacheOnly
		if !outcome.Truncated && !deadlineCutOff {
			return nil
		}
		if pass == len(passModes)-1 {
			// Unreachable if the store honours its contract: cache-only
			// serves are read into memory up front, so a disk failure
			// surfaces as Failed, not Truncated. Kept as insurance; serve
			// what parsed.
			log.Printf("filter lists still truncated after %d passes, continuing with incomplete rules", len(passModes))
			return nil
		}
		if outcome.Truncated {
			log.Printf("truncated filter list detected on pass %d, rebuilding", pass+1)
		} else {
			log.Printf("build deadline expired before every filter list was served on pass %d, rebuilding from cache", pass+1)
		}
	}
	return nil
}

// populateFilter fills f with every enabled filter list plus the user's own
// rules, and reports the merged outcome across all lists. It returns after
// every list has been fetched and parsed. f is left unfinalized: compacting
// a structure that a truncated outcome is about to discard would be wasted
// work, so buildFilter finalizes the accepted pass only.
func (a *App) populateFilter(ctx context.Context, f *filter.Filter, mode filterliststore.FetchMode) filter.Outcome {
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

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/irbis-sh/zen-desktop/internal/asset"
	"github.com/irbis-sh/zen-desktop/internal/certgen"
	"github.com/irbis-sh/zen-desktop/internal/certstore"
	"github.com/irbis-sh/zen-desktop/internal/config"
	"github.com/irbis-sh/zen-desktop/internal/constants"
	"github.com/irbis-sh/zen-desktop/internal/filter/whitelistserver"
	"github.com/irbis-sh/zen-desktop/internal/filterliststore"
	"github.com/irbis-sh/zen-desktop/internal/logger"
	"github.com/irbis-sh/zen-desktop/internal/proxy"
	"github.com/irbis-sh/zen-desktop/internal/routing"
	"github.com/irbis-sh/zen-desktop/internal/selfupdate"
	"github.com/irbis-sh/zen-desktop/internal/sysproxy"
	"github.com/irbis-sh/zen-desktop/internal/systray"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
	// name is the name of the application.
	name string
	// startupDone is closed once the application has fully started.
	// It ensures that all dependencies are fully initialized
	// before frontend-bound methods can use them.
	startupDone        chan struct{}
	startOnDomReady    bool
	config             *config.Config
	frontendEvents     *frontendEvents
	proxy              *proxy.Proxy
	proxyOn            bool
	systemProxyManager *sysproxy.Manager
	// proxyMu ensures that proxy is only started or stopped once at a time.
	proxyMu sync.Mutex
	// buildAbort cancels the in-flight filter build; nil when no build is
	// running. proxyMu serialises builds, so buildAbortMu only guards against
	// StopProxy reading the func while buildFilter sets or clears it.
	buildAbortMu sync.Mutex
	buildAbort   context.CancelCauseFunc
	// stopPending is a best-effort latch for StartProxy calls already queued
	// on proxyMu when a StopProxy arrives: the abort can only reach the build
	// in flight, and a queued start winning the mutex ahead of the stop would
	// otherwise run a fresh build the stop then waits out. Best-effort
	// because overlapping stops can clear each other's flag; the window this
	// closes is the common one.
	stopPending     atomic.Bool
	certStore       *certstore.DiskCertStore
	systrayMgr      *systray.Manager
	filterListStore *filterliststore.FilterListStore
	whitelistSrv    *whitelistserver.Server
	restartMu       sync.Mutex
	// pendingRestart records an armed restart.
	pendingRestart      bool
	pendingRestartStart bool
}

// NewApp initializes the app.
func NewApp(name string, appConfig *config.Config, startOnDomReady bool) (*App, error) {
	if name == "" {
		return nil, errors.New("name is empty")
	}
	if appConfig == nil {
		return nil, errors.New("config is nil")
	}

	certStore, err := certstore.NewDiskCertStore(appConfig, config.DataDir, constants.OrgName)
	if err != nil {
		return nil, fmt.Errorf("failed to create cert store: %v", err)
	}

	cacheDir, err := config.GetCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache dir: %v", err)
	}
	filterListStore, err := filterliststore.New(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create filter list store: %v", err)
	}

	systemProxyManager := sysproxy.NewManager(appConfig.GetPACPort())

	return &App{
		name:               name,
		startupDone:        make(chan struct{}),
		config:             appConfig,
		certStore:          certStore,
		startOnDomReady:    startOnDomReady,
		systemProxyManager: systemProxyManager,
		filterListStore:    filterListStore,
	}, nil
}

// commonStartup defines startup procedures common to all platforms.
func (a *App) commonStartup(ctx context.Context) {
	a.ctx = ctx

	systrayMgr, err := systray.NewManager(a.name, func() {
		a.StartProxy()
	}, func() {
		a.StopProxy()
	})
	if err != nil {
		log.Fatalf("failed to initialize systray manager: %v", err)
	}

	a.systrayMgr = systrayMgr
	a.frontendEvents = newFrontendEvents(ctx)
	a.config.RunMigrations()
	a.systrayMgr.Init(ctx)

	su, err := selfupdate.NewSelfUpdater(a.config, a.frontendEvents)
	if err != nil {
		log.Printf("failed to initialize self-updater: %v", err)
	} else if su != nil {
		go su.RunScheduledUpdateChecks()
	}

	time.AfterFunc(time.Second, func() {
		// This is a workaround for the issue where not all React components are mounted in time.
		// StartProxy requires an active event listener on the frontend to show the user the correct proxy state.
		// TODO: implement a more reliable solution.
		if a.startOnDomReady {
			a.StartProxy()
		}
	})

	close(a.startupDone)
}

// BeforeClose runs when the user asks to quit. Returning true prevents the
// window from closing.
func (a *App) BeforeClose(ctx context.Context) bool {
	log.Println("shutting down")
	if err := a.StopProxy(); err != nil {
		dialog, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:          runtime.QuestionDialog,
			Title:         "Quit error",
			Message:       fmt.Sprintf("We've encountered an error while shutting down the proxy: %v. Do you want to quit anyway?", err),
			Buttons:       []string{"Yes", "No"},
			DefaultButton: "Yes",
			CancelButton:  "No",
		})
		if err != nil {
			return false
		}
		if dialog != "Yes" {
			// The quit was cancelled; don't leave a relaunch armed for a
			// later normal quit.
			a.restartMu.Lock()
			a.pendingRestart = false
			a.restartMu.Unlock()
			return true
		}
		return false
	}
	a.systrayMgr.Quit()
	return false
}

// StartProxy starts the proxy.
func (a *App) StartProxy() (err error) {
	<-a.startupDone
	defer func() {
		// You might see this pattern both in this file and throughout the application.
		// It is used in functions that get called by the frontend, in which case we cannot log the error at the caller level.
		switch {
		case err == nil:
			log.Println("proxy started successfully")
		case errors.Is(err, errBuildAborted):
			log.Println("proxy start aborted")
		default:
			log.Printf("error starting proxy: %v", err)
		}
	}()

	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()

	if a.proxyOn {
		return nil
	}
	if a.stopPending.Load() {
		// A StopProxy is already queued behind us; building would only hand
		// it more to tear down.
		return errBuildAborted
	}

	log.Println("starting proxy")

	a.frontendEvents.OnProxyStarting()
	defer func() {
		switch {
		case err == nil:
			a.frontendEvents.OnProxyStarted()
		case errors.Is(err, errBuildAborted):
			// A deliberate stop, not a failure: no startError, which the
			// frontend surfaces as an error toast. The StopProxy that aborted
			// the build is waiting on proxyMu and emits stopping/stopped as
			// soon as we return, taking the frontend out of its loading state.
		default:
			a.frontendEvents.OnProxyStartError(err)
		}
	}()

	certGenerator, err := certgen.NewCertGenerator(a.certStore, constants.OrgName)
	if err != nil {
		return fmt.Errorf("create cert manager: %v", err)
	}

	filter, whitelistSrv, assetInjector, err := a.buildFilter()
	if err != nil {
		return err
	}

	if err := whitelistSrv.Start(); err != nil {
		return fmt.Errorf("start whitelist server: %v", err)
	}
	a.whitelistSrv = whitelistSrv

	defer func() {
		if err != nil {
			if err := whitelistSrv.Stop(); err != nil {
				log.Printf("failed to stop whitelist server: %v", err)
			}
			a.whitelistSrv = nil
		}
	}()

	routingPolicy := routing.NewPolicy(a.config.GetRouting())

	a.proxy, err = proxy.NewProxy(filter, certGenerator, a.config.GetPort(), routingPolicy.ShouldProxy, constants.LocalEndpointHost, asset.NewHandler(assetInjector))
	if err != nil {
		return fmt.Errorf("create proxy: %v", err)
	}

	if err := a.certStore.Init(); err != nil {
		if !errors.Is(err, certstore.ErrNoSystemTrustStore) {
			return fmt.Errorf("initialize cert store: %v", err)
		}
		// The store initialized successfully, but the CA could only be installed
		// into NSS databases (e.g. on NixOS). Not fatal: warn and keep starting.
		log.Printf("cert store init: %v", err)
		a.frontendEvents.OnCASystemTrustUnavailable(err)
	}

	port, err := a.proxy.Start()
	if err != nil {
		return fmt.Errorf("start proxy: %v", err)
	}

	a.systemProxyManager.SetPACPort(a.config.GetPACPort())
	if err := a.systemProxyManager.Set(port, a.config.GetIgnoredHosts(), routingPolicy.ShouldProxy); err != nil {
		if errors.Is(err, sysproxy.ErrUnsupportedDesktopEnvironment) {
			a.frontendEvents.OnUnsupportedDE(err)
		} else {
			if stopErr := a.proxy.Stop(); stopErr != nil {
				return fmt.Errorf("stop proxy: %v, set system proxy: %v", stopErr, err)
			}
			return fmt.Errorf("set system proxy: %v", err)
		}
	}

	a.proxyOn = true

	a.systrayMgr.OnProxyStarted()

	return nil
}

// StopProxy stops the proxy. When a start is in flight, it aborts the filter
// build rather than waiting out filterBuildTimeout behind proxyMu.
func (a *App) StopProxy() (err error) {
	<-a.startupDone
	defer func() {
		if err != nil {
			log.Printf("error stopping proxy: %v", err)
		} else {
			log.Println("proxy stopped successfully")
		}
	}()

	a.stopPending.Store(true)
	a.abortBuild()

	a.proxyMu.Lock()
	a.stopPending.Store(false)
	defer a.proxyMu.Unlock()

	log.Println("stopping proxy")

	a.frontendEvents.OnProxyStopping()
	defer func() {
		if err != nil {
			a.frontendEvents.OnProxyStopError(err)
		} else {
			a.frontendEvents.OnProxyStopped()
		}
	}()

	if !a.proxyOn {
		return nil
	}

	if err := a.systemProxyManager.Clear(); err != nil {
		if errors.Is(err, sysproxy.ErrUnsupportedDesktopEnvironment) {
			log.Printf("system proxy not cleared (unsupported desktop environment): %v", err)
		} else {
			return fmt.Errorf("clear system proxy: %w", err)
		}
	}

	if err := a.proxy.Stop(); err != nil {
		return fmt.Errorf("stop proxy: %w", err)
	}

	if err := a.whitelistSrv.Stop(); err != nil {
		return fmt.Errorf("stop whitelist server: %w", err)
	}

	a.whitelistSrv = nil
	a.proxy = nil
	a.proxyOn = false

	a.systrayMgr.OnProxyStopped()

	return nil
}

func (a *App) setBuildAbort(abort context.CancelCauseFunc) {
	a.buildAbortMu.Lock()
	defer a.buildAbortMu.Unlock()
	a.buildAbort = abort
}

func (a *App) abortBuild() {
	a.buildAbortMu.Lock()
	defer a.buildAbortMu.Unlock()
	if a.buildAbort != nil {
		a.buildAbort(errBuildAborted)
	}
}

// UninstallCA uninstalls the CA.
func (a *App) UninstallCA() error {
	if err := a.certStore.UninstallCA(); err != nil {
		log.Printf("failed to uninstall CA: %v", err)
		return err
	}

	return nil
}

func (a *App) OpenLogsDirectory() error {
	if err := logger.OpenLogsDirectory(); err != nil {
		log.Printf("failed to open logs directory: %v", err)
		return err
	}

	return nil
}

func (a *App) SelectAppForRouting() (string, error) {
	<-a.startupDone

	const dialogTitle = "Select app"

	switch goruntime.GOOS {
	case "darwin":
		return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
			Title:                      dialogTitle,
			DefaultDirectory:           "/Applications",
			Filters:                    []runtime.FileFilter{{DisplayName: "Applications", Pattern: "*.app"}},
			TreatPackagesAsDirectories: true,
		})
	case "windows":
		return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
			Title:   dialogTitle,
			Filters: []runtime.FileFilter{{DisplayName: "Applications", Pattern: "*.exe"}},
		})
	case "linux":
		return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
			Title: dialogTitle,
		})
	default:
		return "", fmt.Errorf("unsupported platform")
	}
}

// ExportCustomFilterListsToFile exports the custom filter lists to a file.
func (a *App) ExportCustomFilterLists() error {
	<-a.startupDone

	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Custom Filter Lists",
		DefaultFilename: "filter-lists.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})

	if err != nil {
		log.Printf("failed to open file dialog: %v", err)
		return err
	}

	if filePath == "" {
		return errors.New("no file selected")
	}

	customFilterLists := a.config.GetTargetTypeFilterLists(config.FilterListTypeCustom)

	if len(customFilterLists) == 0 {
		return errors.New("no custom filter lists to export")
	}

	data, err := json.MarshalIndent(customFilterLists, "", "  ")
	if err != nil {
		log.Printf("failed to marshal filter lists: %v", err)
		return err
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		log.Printf("failed to write filter lists to file: %v", err)
		return err
	}

	return nil
}

// ImportCustomFilterLists imports the custom filter lists from a file.
func (a *App) ImportCustomFilterLists() error {
	<-a.startupDone

	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Custom Filter Lists",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})

	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("failed to read filter lists file: %v", err)
		return err
	}

	var filterLists []config.FilterList
	if err := json.Unmarshal(data, &filterLists); err != nil {
		log.Printf("failed to unmarshal filter lists: %v", err)
		return errors.New("incorrect filter lists format")
	}

	if len(filterLists) == 0 {
		return errors.New("no custom filter lists to import")
	}

	if err := a.config.AddFilterLists(filterLists); err != nil {
		log.Printf("failed to add filter lists: %v", err)
		return err
	}

	return nil
}

func (a *App) IsNoSelfUpdate() bool {
	return selfupdate.NoSelfUpdate == "true"
}

func (a *App) OnSecondInstanceLaunch(secondInstanceData options.SecondInstanceData) {
	start, hidden := parseLaunchArgs(secondInstanceData.Args)
	if !hidden {
		runtime.WindowUnmaximise(a.ctx)
		runtime.Show(a.ctx)
	}
	if start {
		a.StartProxy()
	}
}

func parseLaunchArgs(args []string) (start, hidden bool) {
	for _, arg := range args {
		if arg == "-start" || arg == "--start" {
			start = true
		}
		if arg == "-hidden" || arg == "--hidden" {
			hidden = true
		}
	}
	return start, hidden
}

// RestartApplication schedules a relaunch of the application and quits.
// The replacement process is spawned by [App.RunPendingRestart] only after the
// Wails runtime has shut down: spawning it here would race the
// single-instance lock, which this process holds until it exits, making the
// new process hand its arguments to this dying one and exit instead of
// starting up. A failure to spawn the replacement is logged, not surfaced
// to the caller.
func (a *App) RestartApplication() error {
	a.proxyMu.Lock()
	proxyOn := a.proxyOn
	a.proxyMu.Unlock()

	a.restartMu.Lock()
	a.pendingRestart, a.pendingRestartStart = true, proxyOn
	a.restartMu.Unlock()

	runtime.Quit(a.ctx)
	return nil
}

// RunPendingRestart spawns the replacement process recorded by
// [App.RestartApplication], if any. It must only be called after wails.Run has
// returned, when this process is about to release the single-instance lock.
func (a *App) RunPendingRestart() {
	a.restartMu.Lock()
	pending, start := a.pendingRestart, a.pendingRestartStart
	a.restartMu.Unlock()
	if !pending {
		return
	}

	// --hidden is deliberately not carried over: a restart is user-initiated,
	// so the window must be visible. Any flag added in future is dropped on
	// restart unless it is handled here.
	var args []string
	if start {
		args = []string{"--start"}
	}
	cmd := exec.Command(os.Args[0], args...) // #nosec G204 G702 -- re-executing our own binary with a fixed argv is ok
	if err := cmd.Start(); err != nil {
		log.Printf("failed to restart application: %v", err)
	}
}

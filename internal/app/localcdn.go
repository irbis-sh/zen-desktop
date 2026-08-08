package app

import (
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/irbis-sh/zen-desktop/internal/config"
	"github.com/irbis-sh/zen-desktop/internal/filter"
	"github.com/irbis-sh/zen-desktop/internal/localcdn"
	"github.com/irbis-sh/zen-desktop/internal/process"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// localcdnFilter combines the ad-blocking filter with the local resource
// engine. Filter-list blocks take precedence over local resource serving:
// when a filter rule blocks a request, the request is never served locally.
type localcdnFilter struct {
	filter *filter.Filter
	local  *localcdn.Engine
	events *frontendEvents
}

func (f *localcdnFilter) HandleRequest(req *http.Request, processInfo process.Info) (*http.Response, error) {
	filterResp, err := f.filter.HandleRequest(req, processInfo)
	if err != nil || filterResp != nil {
		return filterResp, err
	}

	localResp, result, err := f.local.HandleRequest(req)
	if err != nil {
		return nil, err
	}
	if result.Served {
		f.events.OnLocalServed(req.Method, req.URL.String(), req.Header.Get("Referer"), result.Library, result.CDNHost, result.RequestedVersion, result.Version, processInfo)
	} else if result.Blocked {
		f.events.OnLocalBlocked(req.Method, req.URL.String(), req.Header.Get("Referer"), result.CDNHost, processInfo)
	}
	return localResp, nil
}

func (f *localcdnFilter) HandleResponse(req *http.Request, res *http.Response, processInfo process.Info) error {
	if err := f.local.HandleResponse(req, res); err != nil {
		// This error is recoverable; the response is still served.
		log.Printf("error rewriting response for local resources: %v", err)
	}
	return f.filter.HandleResponse(req, res, processInfo)
}

// GetLocalResourcesSettings returns the local resource engine settings.
func (a *App) GetLocalResourcesSettings() config.LocalResources {
	return a.localcdnMgr.GetSettings()
}

// SetLocalResourcesEnabled enables or disables the local resource engine.
func (a *App) SetLocalResourcesEnabled(enabled bool) error {
	return a.localcdnMgr.SetEnabled(enabled)
}

// SetLocalResourcesBlockMissing enables or disables blocking missing resources.
func (a *App) SetLocalResourcesBlockMissing(blockMissing bool) error {
	return a.localcdnMgr.SetBlockMissing(blockMissing)
}

// SetLocalResourcesCustomDir sets the custom resource directory.
func (a *App) SetLocalResourcesCustomDir(dir string) error {
	return a.localcdnMgr.SetCustomDir(dir)
}

// SetLocalResourcesLibraryEnabled enables or disables a single library.
func (a *App) SetLocalResourcesLibraryEnabled(key string, enabled bool) error {
	return a.localcdnMgr.SetLibraryEnabled(key, enabled)
}

// GetLocalResourcesLibraries returns the bundled libraries with their state.
func (a *App) GetLocalResourcesLibraries() []localcdn.LibraryInfo {
	return a.localcdnMgr.GetLibraries()
}

// GetLocalResourcesStats returns the local resource injection counters.
func (a *App) GetLocalResourcesStats() config.LocalResourcesStats {
	return a.localcdnMgr.GetStats()
}

// ResetLocalResourcesStats resets the since-reset injection counters.
func (a *App) ResetLocalResourcesStats() error {
	if err := a.localcdnMgr.ResetStats(); err != nil {
		return err
	}
	if a.frontendEvents != nil {
		// Notify mounted counter components so they refresh immediately.
		a.frontendEvents.emit("localcdn:stats", struct{}{})
	}
	return nil
}

// ExportLocalResourcesMappings exports the custom resource mappings to a file.
func (a *App) ExportLocalResourcesMappings() error {
	<-a.startupDone

	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Custom Resource Mappings",
		DefaultFilename: "local-resources-mappings.json",
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

	data, err := a.localcdnMgr.ExportMappings()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filePath, []byte(data), 0644); err != nil {
		log.Printf("failed to write custom resource mappings: %v", err)
		return err
	}
	return nil
}

// ImportLocalResourcesMappings imports custom resource mappings from a file.
func (a *App) ImportLocalResourcesMappings() error {
	<-a.startupDone

	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Custom Resource Mappings",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("failed to read custom resource mappings: %v", err)
		return err
	}
	return a.localcdnMgr.ImportMappings(string(data))
}

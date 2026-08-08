package localcdn

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/irbis-sh/zen-desktop/internal/config"
)

var (
	staticRegistryOnce sync.Once
	staticRegistry     *Registry
	staticRegistryErr  error
)

// bundledRegistry returns the registry built from the embedded manifest,
// cached across calls.
func bundledRegistry() (*Registry, error) {
	staticRegistryOnce.Do(func() {
		staticRegistry, staticRegistryErr = NewRegistry(resourcesManifest, nil)
	})
	return staticRegistry, staticRegistryErr
}

// CDNHosts returns the exact CDN hosts bundled with the engine. These hosts
// must always be routed through the proxy (see sysproxy renderPac).
func CDNHosts() []string {
	registry, err := bundledRegistry()
	if err != nil {
		return nil
	}
	return registry.CDNHosts()
}

// IsKnownCDNHost reports whether the host is a CDN host with bundled local
// resources.
func IsKnownCDNHost(host string) bool {
	registry, err := bundledRegistry()
	if err != nil {
		return false
	}
	return registry.isKnownCDNHost(host)
}

// Manager exposes local resource settings and statistics to the frontend via
// Wails bindings.
type Manager struct {
	config *config.Config
	stats  *Stats
}

// NewManager creates a Manager backed by the given application config.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		config: cfg,
		stats:  newStats(cfg.GetLocalResourcesStats()),
	}
}

// Stats returns the live stats object shared with the proxy engine.
func (m *Manager) Stats() *Stats {
	return m.stats
}

// GetSettings returns the current local resource settings.
func (m *Manager) GetSettings() config.LocalResources {
	return m.config.GetLocalResources()
}

// SetEnabled enables or disables the local resource engine.
func (m *Manager) SetEnabled(enabled bool) error {
	return m.config.SetLocalResourcesEnabled(enabled)
}

// SetBlockMissing enables or disables blocking requests for missing resources.
func (m *Manager) SetBlockMissing(blockMissing bool) error {
	return m.config.SetLocalResourcesBlockMissing(blockMissing)
}

// SetCustomDir sets the custom resource directory.
func (m *Manager) SetCustomDir(dir string) error {
	return m.config.SetLocalResourcesCustomDir(dir)
}

// SetLibraryEnabled enables or disables a single library.
func (m *Manager) SetLibraryEnabled(key string, enabled bool) error {
	return m.config.SetLocalResourcesLibraryEnabled(key, enabled)
}

// GetLibraries returns the bundled libraries with their enabled state.
func (m *Manager) GetLibraries() []LibraryInfo {
	registry, err := bundledRegistry()
	if err != nil {
		return nil
	}
	return registry.LibraryInfos(m.config.GetLocalResources().EnabledLibraries)
}

// GetStats returns a snapshot of the live injection counters.
func (m *Manager) GetStats() config.LocalResourcesStats {
	return m.stats.Snapshot()
}

// ResetStats resets the since-reset counters.
func (m *Manager) ResetStats() error {
	m.stats.Reset()
	return m.config.ResetLocalResourcesStats()
}

// ExportMappings returns the custom resource mappings as JSON.
func (m *Manager) ExportMappings() (string, error) {
	mappings := m.config.GetLocalResources().CustomMappings
	if len(mappings) == 0 {
		return "", errors.New("no custom resource mappings to export")
	}
	data, err := json.MarshalIndent(mappings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal custom mappings: %w", err)
	}
	return string(data), nil
}

// ImportMappings parses custom resource mappings from JSON and stores them.
func (m *Manager) ImportMappings(data string) error {
	var mappings []config.LocalResourceMapping
	if err := json.Unmarshal([]byte(data), &mappings); err != nil {
		return errors.New("incorrect custom mappings format")
	}
	if len(mappings) == 0 {
		return errors.New("no custom resource mappings to import")
	}
	for i := range mappings {
		if err := validateMapping(&mappings[i]); err != nil {
			return err
		}
		if mappings[i].ID == "" {
			mappings[i].ID = fmt.Sprintf("custom-%d", i+1)
		}
		if mappings[i].Library == "" {
			mappings[i].Library = "custom"
		}
	}
	return m.config.SetLocalResourcesCustomMappings(mappings)
}

// validateMapping checks that a custom mapping has the required fields.
func validateMapping(mapping *config.LocalResourceMapping) error {
	if len(mapping.Patterns) == 0 {
		return errors.New("mapping has no URL patterns")
	}
	if mapping.File == "" {
		return errors.New("mapping has no local file")
	}
	if mapping.ContentType == "" {
		return errors.New("mapping has no content type")
	}
	return nil
}

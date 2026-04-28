package config

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/irbis-sh/zen-desktop/internal/constants"
)

var (
	// ConfigDir is the path to the directory storing the application configuration.
	ConfigDir string
	// DataDir is the path to the directory storing the application data.
	DataDir string
	// Version is the current version of the application. Set at compile time for production builds using ldflags (see tasks in the /tasks/build directory).
	Version = "development"
)

//go:embed default-config.json
var defaultConfig embed.FS

type UpdatePolicyType string

const (
	UpdatePolicyAutomatic UpdatePolicyType = "automatic"
	UpdatePolicyPrompt    UpdatePolicyType = "prompt"
	UpdatePolicyDisabled  UpdatePolicyType = "disabled"
)

var UpdatePolicyEnum = []struct {
	Value  UpdatePolicyType
	TSName string
}{
	{UpdatePolicyAutomatic, "AUTOMATIC"},
	{UpdatePolicyPrompt, "PROMPT"},
	{UpdatePolicyDisabled, "DISABLED"},
}

type RoutingMode string

const (
	RoutingModeBlocklist RoutingMode = "blocklist"
	RoutingModeAllowlist RoutingMode = "allowlist"
)

var RoutingModeEnum = []struct {
	Value  RoutingMode
	TSName string
}{
	{RoutingModeBlocklist, "BLOCKLIST"},
	{RoutingModeAllowlist, "ALLOWLIST"},
}

type RoutingConfig struct {
	Mode     RoutingMode `json:"mode"`
	AppPaths []string    `json:"appPaths"`
}

type FilterListType string

const (
	FilterListTypeGeneral          FilterListType = "general"
	FilterListTypeAds              FilterListType = "ads"
	FilterListTypePrivacy          FilterListType = "privacy"
	FilterListTypeMalware          FilterListType = "malware"
	FilterListTypeRegional         FilterListType = "regional"
	FilterListTypeDigitalWellbeing FilterListType = "digitalWellbeing"
	FilterListTypeCustom           FilterListType = "custom"
)

type FilterList struct {
	Name    string         `json:"name"`
	Type    FilterListType `json:"type"`
	URL     string         `json:"url"`
	Enabled bool           `json:"enabled"`
	Trusted bool           `json:"trusted"`
	Locales []string       `json:"locales"`
}

// Config stores and manages the configuration for the application.
// Although all fields are public, this is only for use by the JSON marshaller.
// All access to the Config should be done through the exported methods.
type Config struct {
	mu sync.RWMutex

	Filter struct {
		FilterLists []FilterList `json:"filterLists"`
		// Deprecated: use Rules.
		MyRules   []string `json:"myRules"`
		Rules     []string `json:"rules"`
		AssetPort int      `json:"assetPort"`
	} `json:"filter"`
	Certmanager struct {
		CAInstalled bool `json:"caInstalled"`
	} `json:"certmanager"`
	Proxy struct {
		Port         int           `json:"port"`
		IgnoredHosts []string      `json:"ignoredHosts"`
		PACPort      int           `json:"pacPort"`
		Routing      RoutingConfig `json:"routing"`
	} `json:"proxy"`
	UpdatePolicy UpdatePolicyType `json:"updatePolicy"`

	Locale string `json:"locale"`

	// firstLaunch is true if the application is being run for the first time.
	firstLaunch bool
}

type DebugData struct {
	EnabledFilterListURLs []string `json:"enabledFilterListURLs"`
	Rules                 []string `json:"rules"`
	Platform              string   `json:"platform"`
	Architecture          string   `json:"architecture"`
	Version               string   `json:"version"`
}

func (c *Config) ExportDebugData() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var enabledFilterListURLs []string
	for _, filterList := range c.Filter.FilterLists {
		if filterList.Enabled {
			enabledFilterListURLs = append(enabledFilterListURLs, filterList.URL)
		}
	}
	debugData := DebugData{
		EnabledFilterListURLs: enabledFilterListURLs,
		Rules:                 c.Filter.Rules,
		Platform:              runtime.GOOS,
		Architecture:          runtime.GOARCH,
		Version:               Version,
	}
	jsonData, err := json.MarshalIndent(debugData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal debug data: %w", err)
	}
	return string(jsonData), nil
}

func init() {
	var err error
	ConfigDir, err = getConfigDir()
	if err != nil {
		log.Fatalf("failed to get config dir: %v", err)
	}
	stat, err := os.Stat(ConfigDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(ConfigDir, 0755); err != nil {
			log.Fatalf("failed to create config dir: %v", err)
		}
		stat, err = os.Stat(ConfigDir)
	}
	if err != nil {
		log.Fatalf("failed to stat config dir: %v", err)
	}
	if !stat.IsDir() {
		log.Fatalf("config dir is not a directory: %s", ConfigDir)
	}

	DataDir, err = getDataDir()
	if err != nil {
		log.Fatalf("failed to get data dir: %v", err)
	}
	stat, err = os.Stat(DataDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(DataDir, 0755); err != nil {
			log.Fatalf("failed to create data dir: %v", err)
		}
		stat, err = os.Stat(DataDir)
	}
	if err != nil {
		log.Fatalf("failed to stat data dir: %v", err)
	}
	if !stat.IsDir() {
		log.Fatalf("data dir is not a directory: %s", DataDir)
	}
}

func New() (*Config, error) {
	c := &Config{}

	configFile := filepath.Join(ConfigDir, "config.json")
	var configData []byte
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		configData, err = os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %v", err)
		}
	} else {
		configData, err = defaultConfig.ReadFile("default-config.json")
		if err != nil {
			return nil, fmt.Errorf("failed to read default config file: %v", err)
		}
		if err := os.WriteFile(configFile, configData, 0644); err != nil {
			return nil, fmt.Errorf("failed to write config file: %v", err)
		}
		c.firstLaunch = true
	}

	if err := json.Unmarshal(configData, c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}

	return c, nil
}

// GetFilterLists returns the list of enabled filter lists.
func (c *Config) GetFilterLists() []FilterList {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Filter.FilterLists
}

// AddFilterList adds a new filter list to the list of enabled filter lists.
func (c *Config) AddFilterList(list FilterList) error {
	return c.update(func() error {
		for _, existingList := range c.Filter.FilterLists {
			if existingList.URL == list.URL {
				return fmt.Errorf("filter list with the URL '%s' already exists", list.URL)
			}
		}

		c.Filter.FilterLists = append(c.Filter.FilterLists, list)
		return nil
	})
}

func (c *Config) AddFilterLists(lists []FilterList) error {
	return c.update(func() error {
		c.Filter.FilterLists = append(c.Filter.FilterLists, lists...)
		return nil
	})
}

// RemoveFilterList removes a filter list from the list of enabled filter lists.
func (c *Config) RemoveFilterList(url string) error {
	return c.update(func() error {
		for i, filterList := range c.Filter.FilterLists {
			if filterList.URL == url {
				c.Filter.FilterLists = append(c.Filter.FilterLists[:i], c.Filter.FilterLists[i+1:]...)
				break
			}
		}
		return nil
	})
}

// ToggleFilterList toggles the enabled state of a filter list.
func (c *Config) ToggleFilterList(url string, enabled bool) error {
	return c.update(func() error {
		for i, filterList := range c.Filter.FilterLists {
			if filterList.URL == url {
				c.Filter.FilterLists[i].Enabled = enabled
				break
			}
		}
		return nil
	})
}

// GetTargetTypeFilterLists returns the list of filter lists with particular type.
func (c *Config) GetTargetTypeFilterLists(targetType FilterListType) []FilterList {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var filterLists []FilterList
	for _, filterList := range c.Filter.FilterLists {
		if filterList.Type == targetType {
			filterLists = append(filterLists, filterList)
		}
	}
	return filterLists
}

func (c *Config) GetFilterListsByLocales(searchLocales []string) []FilterList {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(searchLocales) == 0 {
		return nil
	}

	exactLocales := make(map[string]struct{}, len(searchLocales))
	langLocales := make(map[string]struct{}, len(searchLocales))
	for _, locale := range searchLocales {
		locale = strings.TrimSpace(locale)
		if locale == "" {
			continue
		}
		exactLocales[locale] = struct{}{}
		if dash := strings.IndexByte(locale, '-'); dash != -1 {
			langLocales[locale[:dash]] = struct{}{}
		} else {
			langLocales[locale] = struct{}{}
		}
	}

	var filterLists []FilterList
outer:
	for _, filterList := range c.Filter.FilterLists {
		for _, locale := range filterList.Locales {
			if strings.IndexByte(locale, '-') != -1 {
				if _, ok := exactLocales[locale]; ok {
					filterLists = append(filterLists, filterList)
					continue outer
				}
			} else {
				if _, ok := langLocales[locale]; ok {
					filterLists = append(filterLists, filterList)
					continue outer
				}
			}
		}
	}
	return filterLists
}

func (c *Config) GetRules() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Filter.Rules
}

func (c *Config) SetRules(rules []string) error {
	return c.update(func() error {
		c.Filter.Rules = rules
		return nil
	})
}

// GetPort returns the port the proxy is set to listen on.
func (c *Config) GetPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Proxy.Port
}

// SetPort sets the port the proxy is set to listen on.
func (c *Config) SetPort(port int) error {
	return c.update(func() error {
		c.Proxy.Port = port
		return nil
	})
}

// GetIgnoredHosts returns the list of ignored hosts.
func (c *Config) GetIgnoredHosts() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Proxy.IgnoredHosts
}

// SetIgnoredHosts sets the list of ignored hosts.
func (c *Config) SetIgnoredHosts(hosts []string) error {
	return c.update(func() error {
		c.Proxy.IgnoredHosts = hosts
		return nil
	})
}

func (c *Config) GetRouting() RoutingConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Proxy.Routing
}

func (c *Config) SetRouting(routing RoutingConfig) error {
	dedupedPaths := make([]string, 0, len(routing.AppPaths))
	seen := make(map[string]struct{}, len(routing.AppPaths))
	for _, path := range routing.AppPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		dedupedPaths = append(dedupedPaths, path)
	}
	routing.AppPaths = dedupedPaths

	return c.update(func() error {
		c.Proxy.Routing = routing
		return nil
	})
}

// GetCAInstalled returns whether the CA is installed.
func (c *Config) GetCAInstalled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Certmanager.CAInstalled
}

// SetCAInstalled sets whether the CA is installed.
func (c *Config) SetCAInstalled(caInstalled bool) {
	_ = c.update(func() error {
		c.Certmanager.CAInstalled = caInstalled
		return nil
	})
}

func (c *Config) GetPACPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Proxy.PACPort
}

// GetAssetPort returns the port the asset server is set to listen on.
func (c *Config) GetAssetPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Filter.AssetPort
}

// SetAssetPort sets the port the asset server is set to listen on.
func (c *Config) SetAssetPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	return c.update(func() error {
		c.Filter.AssetPort = port
		return nil
	})
}

func (c *Config) GetVersion() string {
	return Version
}

func (c *Config) GetUpdatePolicy() UpdatePolicyType {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.UpdatePolicy
}

func (c *Config) SetUpdatePolicy(p UpdatePolicyType) error {
	return c.update(func() error {
		c.UpdatePolicy = p
		return nil
	})
}

func (c *Config) GetLocale() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Locale
}

func (c *Config) SetLocale(l string) error {
	return c.update(func() error {
		c.Locale = l
		return nil
	})
}

func (c *Config) GetFirstLaunch() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.firstLaunch
}

func GetCacheDir() (string, error) {
	var appName string
	switch runtime.GOOS {
	case "darwin", "windows":
		appName = constants.AppName
	case "linux":
		appName = constants.AppNameLowercase
	default:
		panic("unsupported platform")
	}

	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName, "filters"), nil
}

// update wraps config update operations. It acquires the config lock,
// executes the provided callback, and then saves the config.
//
// Any error encountered is returned to the caller. The callback is expected
// to modify the config and must not acquire the lock itself.
func (c *Config) update(fn func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := fn(); err != nil {
		log.Printf("config update failed: %v", err)
		return err
	}
	if err := c.saveLocked(); err != nil {
		err = fmt.Errorf("save config: %w", err)
		log.Printf("config update failed: %v", err)
		return err
	}
	return nil
}

// saveLocked saves the config to disk.
// The caller must hold c's write lock.
func (c *Config) saveLocked() error {
	configData, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	configFile := filepath.Join(ConfigDir, "config.json")
	err = os.WriteFile(configFile, configData, 0644)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/blang/semver"
	"github.com/irbis-sh/zen-desktop/internal/autostart"
)

type migration struct {
	version string
	fn      func(c *Config) error
}

// migrations is an ordered list of version migrations.
// Maintainers: always append new migrations to the end of this slice.
// Do not reorder existing entries - migrations run sequentially from first to last.
var migrations = []migration{
	{"v0.3.0", func(c *Config) error {
		if err := c.AddFilterList(FilterList{
			Name:    "DandelionSprout's URL Shortener",
			Type:    "privacy",
			URL:     "https://raw.githubusercontent.com/DandelionSprout/adfilt/master/LegitimateURLShortener.txt",
			Enabled: true,
		}); err != nil {
			return err
		}
		return nil
	}},
	{"v0.6.0", func(c *Config) error {
		// https://github.com/irbis-sh/zen-desktop/issues/146
		if err := c.ToggleFilterList("https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_2_Base/filter.txt", true); err != nil {
			return err
		}
		return nil
	}},
	{"v0.7.0", func(c *Config) error {
		// https://github.com/irbis-sh/zen-desktop/issues/147#issuecomment-2521317897
		return c.update(func() error {
			for i, list := range c.Filter.FilterLists {
				if list.URL == "https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_2_Base/filter.txt" || list.URL == "https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_3_Spyware/filter.txt" {
					c.Filter.FilterLists[i].Trusted = true
					log.Printf("v0.7.0 migration: setting %q list as trusted", list.URL)
				}
				if list.URL == "https://easylist-downloads.adblockplus.org/easylist_noelemhide.txt" {
					c.Filter.FilterLists[i].URL = "https://easylist.to/easylist/easylist.txt"
					log.Printf("v0.7.0 migration: updating EasyList's URL")
				}
			}
			return nil
		})
	}},
	{"v0.9.0", func(c *Config) error {
		if err := c.update(func() error {
			c.UpdatePolicy = UpdatePolicyPrompt
			return nil
		}); err != nil {
			return err
		}

		if runtime.GOOS != "darwin" {
			autostart := autostart.Manager{}
			if enabled, err := autostart.IsEnabled(); err != nil {
				return fmt.Errorf("check enabled: %w", err)
			} else if enabled {
				// Re-enable to change autostart command
				if err := autostart.Disable(); err != nil {
					return fmt.Errorf("disable autostart: %w", err)
				}
				if err := autostart.Enable(); err != nil {
					return fmt.Errorf("enable autostart: %w", err)
				}
			}
		}

		return nil
	}},
	{"v0.10.0", func(c *Config) error {
		if err := c.update(func() error {
			for i, list := range c.Filter.FilterLists {
				if list.URL == "https://raw.githubusercontent.com/hufilter/hufilter/master/hufilter.txt" {
					c.Filter.FilterLists[i].URL = "https://filters.hufilter.hu/hufilter-adguard.txt"
					log.Printf("v0.10.0 migration: updating Hungarian filter list's URL")
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if err := c.AddFilterList(FilterList{
			Name:    "Zen - Ads",
			Type:    "ads",
			URL:     "https://raw.githubusercontent.com/ZenPrivacy/filter-lists/master/ads/ads.txt",
			Enabled: true,
		}); err != nil {
			return fmt.Errorf("add \"Zen - Ads\" filter list: %w", err)
		}
		return nil
	}},
	{"v0.11.0", func(c *Config) error {
		if err := c.update(func() error {
			for i, list := range c.Filter.FilterLists {
				if list.URL == "https://adblock.gardar.net/is.abp.txt" {
					c.Filter.FilterLists[i].URL = "https://raw.githubusercontent.com/brave/adblock-lists/master/custom/is.txt"
					c.Filter.FilterLists[i].Name = "🇮🇸IS: Adblock listi fyrir íslenskar vefsíður"
					log.Printf("v0.11.0 migration: updating the Icelandic list's URL and name")
				}
				if list.URL == "https://easylist-downloads.adblockplus.org/ruadlist.txt" {
					c.Filter.FilterLists[i].URL = "https://raw.githubusercontent.com/dimisa-RUAdList/RUAdListCDN/refs/heads/main/lists/ruadlist.ubo.min.txt"
					log.Printf("v0.11.0 migration: updating the RU AdList's URL")
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if err := c.AddFilterList(FilterList{
			Name:    "Zen - Privacy",
			Type:    "privacy",
			URL:     "https://raw.githubusercontent.com/ZenPrivacy/filter-lists/master/privacy/privacy.txt",
			Enabled: true,
		}); err != nil {
			return fmt.Errorf("add \"Zen - Privacy\" filter list: %w", err)
		}
		return nil
	}},
	{"v0.11.3", func(c *Config) error {
		return c.update(func() error {
			for i, list := range c.Filter.FilterLists {
				if list.URL == "https://malware-filter.gitlab.io/malware-filter/phishing-filter.txt" {
					c.Filter.FilterLists[i].URL = "https://malware-filter.gitlab.io/malware-filter/phishing-filter-hosts.txt"
				}
			}
			return nil
		})
	}},
	{"v0.12.0", func(_ *Config) error {
		if runtime.GOOS == "darwin" {
			autostart := autostart.Manager{}
			enabled, err := autostart.IsEnabled()
			if err != nil {
				return fmt.Errorf("check enabled: %v", err)
			}
			if enabled {
				// Re-enable to update ProgramArguments
				if err := autostart.Disable(); err != nil {
					return fmt.Errorf("disable autostart: %v", err)
				}
				if err := autostart.Enable(); err != nil {
					return fmt.Errorf("enable autostart: %v", err)
				}
			}
		}

		return nil
	}},
	{"v0.13.0", func(c *Config) error {
		if err := c.update(func() error {
			if c.UpdatePolicy == UpdatePolicyPrompt {
				c.UpdatePolicy = UpdatePolicyAutomatic
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}},
	{"v0.16.0", func(c *Config) error {
		return c.update(func() error {
			c.Filter.Rules = c.Filter.MyRules
			return nil
		})
	}},
	{"v0.17.0", func(c *Config) error {
		return c.update(func() error {
			c.Filter.AssetPort = 26514
			return nil
		})
	}},
	{"v0.18.0", func(c *Config) error {
		lists := []FilterList{
			{
				Name:    "no-doomscroll - All",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - YouTube",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/youtube/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - TikTok",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/tiktok/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - Instagram",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/instagram/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - X",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/x/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - Reddit",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/reddit/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - Bluesky",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/bluesky/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - LinkedIn",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/linkedin/main-zen.txt",
				Enabled: false,
			},
			{
				Name:    "no-doomscroll - Twitch",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/twitch/main-zen.txt",
				Enabled: false,
			},
		}
		for _, list := range lists {
			if err := c.AddFilterList(list); err != nil {
				return fmt.Errorf("add no-doomscroll list %q: %w", list.Name, err)
			}
		}
		return nil
	}},
	{"v0.19.0", func(c *Config) error {
		return c.update(func() error {
			const (
				oldZenAdsURL     = "https://raw.githubusercontent.com/ZenPrivacy/filter-lists/master/ads/ads.txt"
				newZenAdsURL     = "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/ads/ads.txt"
				oldZenPrivacyURL = "https://raw.githubusercontent.com/ZenPrivacy/filter-lists/master/privacy/privacy.txt"
				newZenPrivacyURL = "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/privacy/privacy.txt"
				ytShortsURL      = "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/youtube/shorts-zen.txt"
				ytMainURL        = "https://cdn.jsdelivr.net/gh/ZenPrivacy/filter-lists@master/no-doomscroll/youtube/main-zen.txt"
			)

			// Remove all variants of Zen filter URLs and YouTube Shorts,
			// then re-add with the correct jsDelivr URLs. This covers all
			// cases: first migration (old URLs), repeat run (new URLs),
			// and manual additions (both URLs present).
			zenAdsEnabled := true
			zenPrivacyEnabled := true
			ytShortsEnabled := false
			filtered := c.Filter.FilterLists[:0]
			for _, list := range c.Filter.FilterLists {
				switch list.URL {
				case oldZenAdsURL, newZenAdsURL:
					zenAdsEnabled = list.Enabled
					continue
				case oldZenPrivacyURL, newZenPrivacyURL:
					zenPrivacyEnabled = list.Enabled
					continue
				case ytShortsURL:
					ytShortsEnabled = list.Enabled
					continue
				}
				filtered = append(filtered, list)
			}
			c.Filter.FilterLists = filtered

			// Re-add Zen filter lists with jsDelivr URLs at the top.
			c.Filter.FilterLists = append([]FilterList{
				{
					Name:    "Zen - Ads",
					Type:    FilterListTypeAds,
					URL:     newZenAdsURL,
					Enabled: zenAdsEnabled,
				},
				{
					Name:    "Zen - Privacy",
					Type:    FilterListTypePrivacy,
					URL:     newZenPrivacyURL,
					Enabled: zenPrivacyEnabled,
				},
			}, c.Filter.FilterLists...)
			log.Printf("v0.19.0 migration: added Zen - Ads and Zen - Privacy with jsDelivr URLs")

			// Add YouTube Shorts list right after "no-doomscroll - YouTube".
			shortsList := FilterList{
				Name:    "no-doomscroll - YouTube Shorts",
				Type:    FilterListTypeDigitalWellbeing,
				URL:     ytShortsURL,
				Enabled: ytShortsEnabled,
			}
			inserted := false
			for i, list := range c.Filter.FilterLists {
				if list.URL == ytMainURL {
					c.Filter.FilterLists = slices.Insert(c.Filter.FilterLists, i+1, shortsList)
					inserted = true
					break
				}
			}
			if !inserted {
				c.Filter.FilterLists = append(c.Filter.FilterLists, shortsList)
			}
			log.Printf("v0.19.0 migration: added no-doomscroll - YouTube Shorts list")

			return nil
		})
	}},
	{"v0.21.0", func(c *Config) error {
		return c.update(func() error {
			for i, fl := range c.Filter.FilterLists {
				if strings.HasPrefix(fl.URL, "https://cdn.jsdelivr.net/gh/ZenPrivacy") {
					c.Filter.FilterLists[i].URL = strings.Replace(fl.URL, "https://cdn.jsdelivr.net/gh/ZenPrivacy", "https://cdn.jsdelivr.net/gh/irbis-sh", 1)
				}
			}
			c.Proxy.Routing = RoutingConfig{
				Mode:     RoutingModeBlocklist,
				AppPaths: []string{},
			}
			return nil
		})
	}},
}

// RunMigrations runs the version-to-version migrations in order.
func (c *Config) RunMigrations() {
	if Version == "development" {
		log.Println("skipping migrations in development mode")
		return
	}

	var lastMigration string
	lastMigrationFile := filepath.Join(ConfigDir, "last_migration")
	if c.firstLaunch {
		lastMigration = Version
	} else {
		if _, err := os.Stat(lastMigrationFile); !os.IsNotExist(err) {
			lastMigrationData, err := os.ReadFile(lastMigrationFile)
			if err != nil {
				log.Fatalf("failed to read last migration file: %v", err)
			}
			lastMigration = string(lastMigrationData)
		} else {
			// Should trigger when updating from pre v0.3.0
			lastMigration = "v0.0.0"
		}
	}

	lastMigrationV, err := semver.ParseTolerant(lastMigration)
	if err != nil {
		log.Printf("error parsing last migration(%s): %v", lastMigration, err)
		return
	}

	for _, m := range migrations {
		versionV, err := semver.ParseTolerant(m.version)
		if err != nil {
			log.Printf("error parsing migration version(%s): %v", m.version, err)
			continue
		}

		if lastMigrationV.LT(versionV) {
			if err := m.fn(c); err != nil {
				log.Printf("error running migration(%s): %v", m.version, err)
			} else {
				log.Printf("ran migration %s", m.version)
			}
		}
	}

	if err := os.WriteFile(lastMigrationFile, []byte(Version), 0644); err != nil {
		log.Printf("error writing last migration file: %v", err)
	}
}

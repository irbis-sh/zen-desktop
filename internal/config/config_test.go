package config

import (
	"reflect"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/i18n"
)

// TestLocaleReachesI18n checks that the browser pages follow the locale the user
// picks, both when it changes and after a restart.
func TestLocaleReachesI18n(t *testing.T) {
	prevConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() {
		ConfigDir = prevConfigDir
		i18n.SetLocale("")
	})

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.SetLocale("de"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}
	if got := i18n.Current().Lang(); got != "de-DE" {
		t.Errorf("after SetLocale, i18n locale = %q, want de-DE", got)
	}

	i18n.SetLocale("")
	if _, err := New(); err != nil {
		t.Fatalf("New after restart: %v", err)
	}
	if got := i18n.Current().Lang(); got != "de-DE" {
		t.Errorf("after restart, i18n locale = %q, want de-DE", got)
	}
}

func TestAddFilterListsSkipsExistingURLs(t *testing.T) {
	prevConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() {
		ConfigDir = prevConfigDir
	})

	c := &Config{}
	c.Filter.FilterLists = []FilterList{{Name: "existing", URL: "a"}}

	err := c.AddFilterLists([]FilterList{
		{Name: "imported", URL: "a"},
		{Name: "new", URL: "b"},
		{Name: "new again", URL: "b"},
	})
	if err != nil {
		t.Fatalf("AddFilterLists: %v", err)
	}

	want := []FilterList{
		{Name: "existing", URL: "a"},
		{Name: "new", URL: "b"},
	}
	if !reflect.DeepEqual(c.Filter.FilterLists, want) {
		t.Errorf("got lists %+v, want %+v", c.Filter.FilterLists, want)
	}
}

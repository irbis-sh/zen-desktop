package config

import "testing"

func TestMigrationV0220RemovesStaleAntiAdblockList(t *testing.T) {
	prevConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() {
		ConfigDir = prevConfigDir
	})

	const removedURL = "https://raw.githubusercontent.com/olegwukr/polish-privacy-filters/master/anti-adblock.txt"
	const keepURL = "https://example.com/keep.txt"

	c := &Config{}
	c.Filter.FilterLists = []FilterList{
		{URL: keepURL},
		{URL: removedURL},
		{URL: removedURL},
	}

	var m *migration
	for i := range migrations {
		if migrations[i].version == "v0.22.0" {
			m = &migrations[i]
			break
		}
	}
	if m == nil {
		t.Fatal("v0.22.0 migration not found")
	}

	if err := m.fn(c); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	if len(c.Filter.FilterLists) != 1 {
		t.Fatalf("expected 1 list after migration, got %d", len(c.Filter.FilterLists))
	}
	if c.Filter.FilterLists[0].URL != keepURL {
		t.Fatalf("unexpected list URL after migration: %s", c.Filter.FilterLists[0].URL)
	}
}

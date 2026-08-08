package config

import "testing"

func TestLocalResourcesDefaultEnabled(t *testing.T) {
	t.Parallel()

	t.Run("absent localResources defaults to enabled", func(t *testing.T) {
		t.Parallel()

		c := &Config{}
		setLocalResourcesDefaults(c, []byte(`{"filter":{"assetPort":26514}}`))
		if !c.LocalResources.Enabled {
			t.Fatal("expected local resources to default to enabled")
		}
	})

	t.Run("explicit disabled is honored", func(t *testing.T) {
		t.Parallel()

		c := &Config{}
		setLocalResourcesDefaults(c, []byte(`{"localResources":{"enabled":false}}`))
		if c.LocalResources.Enabled {
			t.Fatal("expected local resources to be disabled")
		}
	})

	t.Run("present section keeps enabled state", func(t *testing.T) {
		t.Parallel()

		c := &Config{}
		setLocalResourcesDefaults(c, []byte(`{"localResources":{}}`))
		if c.LocalResources.Enabled {
			t.Fatal("expected an explicitly present (empty) section to keep enabled=false")
		}
	})
}

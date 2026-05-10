package scriptlet

import (
	"testing"
)

func TestAddRule(t *testing.T) {
	t.Parallel()

	t.Run("parses Adguard-style rule", func(t *testing.T) {
		t.Parallel()

		spyStore := &spyScriptletStore{}
		injector, err := newInjector([]byte{}, spyStore)
		if err != nil {
			t.Fatalf("failed to create injector: %v", err)
		}

		if err := injector.AddRule("example.org#%#//scriptlet('set-constant', 'first', 'false')", false); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if len(spyStore.PrimaryEntries) != 1 {
			t.Fatalf("expected exactly one entry to be collected, got %d", len(spyStore.PrimaryEntries))
		}
		if spyStore.PrimaryEntries[0].HostnamePatterns != "example.org" {
			t.Errorf("expected hostname to be collected, got %q", spyStore.PrimaryEntries[0].HostnamePatterns)
		}

		expectedArgList, err := argList(`'set-constant', 'first', 'false'`).Normalize()
		if err != nil {
			t.Fatalf("failed to normalize arg list: %v", err)
		}

		if spyStore.PrimaryEntries[0].ArgList != expectedArgList {
			t.Errorf("expected first scriptlet to be %v, got %v", expectedArgList, spyStore.PrimaryEntries[0].ArgList)
		}
	})

	t.Run("parses Adguard-style exception rule", func(t *testing.T) {
		t.Parallel()

		spyStore := &spyScriptletStore{}
		injector, err := newInjector([]byte{}, spyStore)
		if err != nil {
			t.Fatalf("failed to create injector: %v", err)
		}

		if err := injector.AddRule("example.org#@%#//scriptlet('set-constant', 'first', 'false')", false); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if len(spyStore.ExceptionEntries) != 1 {
			t.Fatalf("expected exactly one entry to be collected, got %d", len(spyStore.ExceptionEntries))
		}
		if spyStore.ExceptionEntries[0].HostnamePatterns != "example.org" {
			t.Errorf("expected hostname to be collected, got %q", spyStore.ExceptionEntries[0].HostnamePatterns)
		}

		expectedArgList, err := argList(`'set-constant', 'first', 'false'`).Normalize()
		if err != nil {
			t.Fatalf("failed to normalize arg list: %v", err)
		}

		if spyStore.ExceptionEntries[0].ArgList != expectedArgList {
			t.Errorf("expected first scriptlet to be %v, got %v", expectedArgList, spyStore.ExceptionEntries[0].ArgList)
		}
	})

	t.Run("parses uBlock-style rule", func(t *testing.T) {
		t.Parallel()

		spyStore := &spyScriptletStore{}
		injector, err := newInjector([]byte{}, spyStore)
		if err != nil {
			t.Fatalf("failed to create injector: %v", err)
		}

		if err := injector.AddRule("example.org##+js(set-constant, first, false)", false); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if len(spyStore.PrimaryEntries) != 1 {
			t.Fatalf("expected exactly one entry to be collected, got %d", len(spyStore.PrimaryEntries))
		}
		if spyStore.PrimaryEntries[0].HostnamePatterns != "example.org" {
			t.Errorf("expected hostname to be collected, got %q", spyStore.PrimaryEntries[0].HostnamePatterns)
		}

		expectedArgList, err := argList(`set-constant, first, false`).ConvertUboToCanonical().Normalize()
		if err != nil {
			t.Fatalf("failed to normalize arg list: %v", err)
		}

		if spyStore.PrimaryEntries[0].ArgList != expectedArgList {
			t.Errorf("expected first scriptlet to be %v, got %v", expectedArgList, spyStore.PrimaryEntries[0].ArgList)
		}
	})

	t.Run("parses uBlock-style exception rule", func(t *testing.T) {
		t.Parallel()

		spyStore := &spyScriptletStore{}
		injector, err := newInjector([]byte{}, spyStore)
		if err != nil {
			t.Fatalf("failed to create injector: %v", err)
		}

		if err := injector.AddRule("example.org#@#+js(set-constant, first, false)", false); err != nil {
			t.Fatalf("failed to add rule: %v", err)
		}

		if len(spyStore.ExceptionEntries) != 1 {
			t.Fatalf("expected exactly one entry to be collected, got %d", len(spyStore.ExceptionEntries))
		}
		if spyStore.ExceptionEntries[0].HostnamePatterns != "example.org" {
			t.Errorf("expected hostname to be collected, got %q", spyStore.ExceptionEntries[0].HostnamePatterns)
		}

		expectedArgList, err := argList(`set-constant, first, false`).ConvertUboToCanonical().Normalize()
		if err != nil {
			t.Fatalf("failed to normalize arg list: %v", err)
		}

		if spyStore.ExceptionEntries[0].ArgList != expectedArgList {
			t.Errorf("expected first scriptlet to be %v, got %v", expectedArgList, spyStore.ExceptionEntries[0].ArgList)
		}
	})

	t.Run("returns an error on an attempt to add a trusted rule if filterListTrusted = false", func(t *testing.T) {
		t.Parallel()

		injector, err := NewInjectorWithDefaults()
		if err != nil {
			t.Fatalf("failed to create injector: %v", err)
		}

		err = injector.AddRule("example.org#@%#//scriptlet('trusted-test', 'first', 'false')", false)
		if err == nil {
			t.Error("expected an error, got nil")
		}

		err = injector.AddRule("example.org#@#+js(trusted-test)", false)
		if err == nil {
			t.Error("expected an error, got nil")
		}
	})
}

type spyScriptletStore struct {
	PrimaryEntries   []spyScriptletStoreEntry
	ExceptionEntries []spyScriptletStoreEntry
}

type spyScriptletStoreEntry struct {
	HostnamePatterns string
	ArgList          argList
}

func (s *spyScriptletStore) AddPrimaryRule(hostnamePatterns string, scriptlet argList) error {
	s.PrimaryEntries = append(s.PrimaryEntries, spyScriptletStoreEntry{
		HostnamePatterns: hostnamePatterns,
		ArgList:          scriptlet,
	})
	return nil
}

func (s *spyScriptletStore) AddExceptionRule(hostnamePatterns string, scriptlet argList) error {
	s.ExceptionEntries = append(s.ExceptionEntries, spyScriptletStoreEntry{
		HostnamePatterns: hostnamePatterns,
		ArgList:          scriptlet,
	})
	return nil
}

func (s *spyScriptletStore) Get(string) []argList {
	return nil
}

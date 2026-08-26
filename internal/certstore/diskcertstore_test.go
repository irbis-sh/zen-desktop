package certstore

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestInitInstallsCA(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	var nssArg *bool
	cs.installNSSFn = func(systemTrustMissing bool) error {
		nssArg = &systemTrustMissing
		return nil
	}

	if err := cs.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !mgr.installed {
		t.Error("CA should be marked installed")
	}
	if nssArg == nil || *nssArg {
		t.Errorf("installNSS should be called with systemTrustMissing=false, got %v", nssArg)
	}
}

func TestInitNSSOnlyWrapsSentinel(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	cs.installTrustFn = func() error {
		return fmt.Errorf("failed to get system trust store: %w", ErrNoSystemTrustStore)
	}
	var nssArg *bool
	cs.installNSSFn = func(systemTrustMissing bool) error {
		nssArg = &systemTrustMissing
		return nil
	}

	err := cs.Init()
	if !errors.Is(err, ErrNoSystemTrustStore) {
		t.Fatalf("Init should return an error wrapping ErrNoSystemTrustStore, got %v", err)
	}
	if !mgr.installed {
		t.Error("NSS-only install is a success: CA should be marked installed")
	}
	if nssArg == nil || !*nssArg {
		t.Errorf("installNSS should be called with systemTrustMissing=true, got %v", nssArg)
	}
}

func TestInitNSSOnlySuppressedWhenSystemTrusted(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	cs.installTrustFn = func() error {
		return fmt.Errorf("failed to get system trust store: %w", ErrNoSystemTrustStore)
	}
	cs.caTrustedBySystemFn = func() bool { return true }

	if err := cs.Init(); err != nil {
		t.Fatalf("Init should not warn when the CA is in the system pool, got %v", err)
	}
	if !mgr.installed {
		t.Error("CA should be marked installed")
	}
}

func TestInitTotalFailureDoesNotWrapSentinel(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	cs.installTrustFn = func() error {
		return fmt.Errorf("failed to get system trust store: %w", ErrNoSystemTrustStore)
	}
	cs.installNSSFn = func(bool) error { return errors.New("no certutil found") }

	err := cs.Init()
	if err == nil {
		t.Fatal("Init should fail when both trust paths fail")
	}
	// Wrapping the sentinel here would make total failure look like the
	// benign NSS-only fallback to StartProxy, which would keep going.
	if errors.Is(err, ErrNoSystemTrustStore) {
		t.Fatalf("total failure must not wrap ErrNoSystemTrustStore, got %v", err)
	}
	if mgr.installed {
		t.Error("CA should not be marked installed")
	}
}

func TestInitTrustFailureIsFatal(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	cs.installTrustFn = func() error { return errors.New("pkexec: authorization could not be obtained") }

	err := cs.Init()
	if err == nil {
		t.Fatal("Init should fail when the trust store exists but the install fails")
	}
	if errors.Is(err, ErrNoSystemTrustStore) {
		t.Fatalf("unexpected sentinel in error: %v", err)
	}
	if mgr.installed {
		t.Error("CA should not be marked installed")
	}
}

func TestInitReusesCAAfterFailedInstall(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	cs.installTrustFn = func() error { return errors.New("install failed") }

	if err := cs.Init(); err == nil {
		t.Fatal("first Init should fail")
	}
	serial := cs.cert.SerialNumber

	cs.installTrustFn = func() error { return nil }
	if err := cs.Init(); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if cs.cert.SerialNumber.Cmp(serial) != 0 {
		t.Error("CA should be reused across a failed install, not regenerated")
	}
}

func TestInitReportsMissingTrustStoreWhenAlreadyInstalled(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	if err := cs.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := cs.Init()
	if err != nil {
		t.Fatalf("second Init with an available trust store: %v", err)
	}

	cs.probeTrustFn = func() error { return ErrNoSystemTrustStore }
	err = cs.Init()
	if !errors.Is(err, ErrNoSystemTrustStore) {
		t.Fatalf("Init should re-report the missing trust store on every call, got %v", err)
	}

	cs.caTrustedBySystemFn = func() bool { return true }
	if err := cs.Init(); err != nil {
		t.Fatalf("Init should not warn once the CA is in the system pool, got %v", err)
	}
}

func TestUninstallCAToleratesMissingTrustStore(t *testing.T) {
	t.Parallel()

	mgr := &fakeCAStatusManager{}
	cs := newTestStore(t, mgr)
	if err := cs.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	cs.uninstallTrustFn = func() error {
		return fmt.Errorf("failed to get system trust store: %w", ErrNoSystemTrustStore)
	}
	if err := cs.UninstallCA(); err != nil {
		t.Fatalf("UninstallCA: %v", err)
	}
	if mgr.installed {
		t.Error("CA should be marked uninstalled")
	}
	if _, err := os.Stat(cs.folderPath); !os.IsNotExist(err) {
		t.Errorf("certs folder should be removed, stat err: %v", err)
	}
}

type fakeCAStatusManager struct {
	installed bool
}

func (m *fakeCAStatusManager) GetCAInstalled() bool      { return m.installed }
func (m *fakeCAStatusManager) SetCAInstalled(value bool) { m.installed = value }

// newTestStore returns a store with all platform trust operations stubbed out
// as successful; tests override individual seams as needed.
func newTestStore(t *testing.T, mgr *fakeCAStatusManager) *DiskCertStore {
	t.Helper()

	cs, err := NewDiskCertStore(mgr, t.TempDir(), "Zen Test")
	if err != nil {
		t.Fatalf("NewDiskCertStore: %v", err)
	}
	cs.installTrustFn = func() error { return nil }
	cs.uninstallTrustFn = func() error { return nil }
	cs.installNSSFn = func(bool) error { return nil }
	cs.uninstallNSSFn = func() error { return nil }
	cs.probeTrustFn = func() error { return nil }
	cs.caTrustedBySystemFn = func() bool { return false }
	return cs
}

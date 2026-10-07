//go:build darwin || windows

package certstore

import (
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/config"
	"github.com/irbis-sh/zen-desktop/internal/hwkey"
)

// These tests use the platform key store: in development builds that is the login keychain on
// macOS and the software key storage provider on Windows. Each test uses its own key name, so
// they cannot touch Zen's real key.

func TestHardwareInitCreatesAndReloadsCA(t *testing.T) {
	cs, mgr, _ := newHardwareTestStore(t)

	if err := cs.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !mgr.installed {
		t.Error("CA should be marked installed")
	}
	ca, err := cs.GetCA()
	if err != nil {
		t.Fatalf("GetCA: %v", err)
	}
	if !ca.HardwareBacked {
		t.Error("CA should be hardware-backed")
	}
	if ca.Cert.MaxPathLen != 1 {
		t.Errorf("root MaxPathLen = %d, want 1 so it can sign an intermediate", ca.Cert.MaxPathLen)
	}
	if _, err := os.Stat(cs.keyPath); !os.IsNotExist(err) {
		t.Errorf("no key file should be written, stat err: %v", err)
	}

	// The installed path opens the key by name and checks it against the certificate.
	if err := cs.Init(); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	reloaded, err := cs.GetCA()
	if err != nil {
		t.Fatalf("GetCA: %v", err)
	}
	if reloaded.Cert.SerialNumber.Cmp(ca.Cert.SerialNumber) != 0 {
		t.Error("second Init should load the existing CA, not create a new one")
	}
}

func TestHardwareInitFailsWhenKeyIsGone(t *testing.T) {
	cs, mgr, name := newHardwareTestStore(t)
	if err := cs.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := hwkey.Delete(name); err != nil {
		t.Fatalf("delete key: %v", err)
	}
	err := cs.Init()
	if err == nil || !strings.Contains(err.Error(), "Uninstall the CA in Settings") {
		t.Fatalf("Init should fail with the uninstall hint, got %v", err)
	}
	if _, err := cs.GetCA(); err == nil {
		t.Error("GetCA should fail after a failed load")
	}

	// A half-deleted CA can still be uninstalled.
	if err := cs.UninstallCA(); err != nil {
		t.Fatalf("UninstallCA: %v", err)
	}
	if mgr.installed {
		t.Error("CA should be marked uninstalled")
	}
}

func TestHardwareInitFailsWhenKeyIsReplaced(t *testing.T) {
	cs, _, name := newHardwareTestStore(t)
	if err := cs.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	replacement, err := hwkey.Create(name)
	if err != nil {
		t.Fatalf("replace key: %v", err)
	}
	replacement.Close()
	if err := cs.Init(); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Init should reject a key that does not match the certificate, got %v", err)
	}
	if _, err := cs.GetCA(); err == nil {
		t.Error("GetCA should fail after a failed load")
	}
}

func TestHardwareUninstallDeletesKey(t *testing.T) {
	cs, _, name := newHardwareTestStore(t)
	if err := cs.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := cs.UninstallCA(); err != nil {
		t.Fatalf("UninstallCA: %v", err)
	}
	if _, err := hwkey.Open(name); !errors.Is(err, hwkey.ErrNotFound) {
		t.Errorf("key should be deleted, Open returned %v", err)
	}
	if _, err := os.Stat(cs.folderPath); !os.IsNotExist(err) {
		t.Errorf("certs folder should be removed, stat err: %v", err)
	}
}

func TestHardwareDiscardKeyAfterFailedInstall(t *testing.T) {
	cs, mgr, name := newHardwareTestStore(t)
	// Init creates the key before installing trust, so a failed trust install leaves the key.
	cs.installTrustFn = func() error { return errors.New("user cancelled") }
	cs.systemTrustAvailableFn = func() bool { return true }
	if err := cs.Init(); err == nil {
		t.Fatal("Init should fail when the trust install fails")
	}
	if mgr.installed {
		t.Fatal("CA should not be marked installed")
	}

	if err := cs.DiscardKey(); err != nil {
		t.Fatalf("DiscardKey: %v", err)
	}
	if _, err := hwkey.Open(name); !errors.Is(err, hwkey.ErrNotFound) {
		t.Errorf("key should be deleted, Open returned %v", err)
	}
	// A second call finds nothing to delete, which is not an error.
	if err := cs.DiscardKey(); err != nil {
		t.Errorf("second DiscardKey: %v", err)
	}
}

// newHardwareTestStore returns a store set to hardware key storage, with a key name unique to
// the test. The key is deleted when the test ends.
func newHardwareTestStore(t *testing.T) (*DiskCertStore, *fakeCAStatusManager, string) {
	t.Helper()

	mgr := &fakeCAStatusManager{storage: config.KeyStorageHardware}
	cs := newTestStore(t, mgr)
	name := "net.zenprivacy.zen.test." + rand.Text()
	cs.hardwareKeyName = name
	t.Cleanup(func() { _ = hwkey.Delete(name) })
	return cs, mgr, name
}

package sysproxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
)

// These tests bind real ports and are deliberately not parallel: parallel runs
// only add freePort collisions, and on macOS a loaded loopback can make Dial to
// an unbound port briefly succeed.

func TestClearClosesServerOnUnsupportedDE(t *testing.T) {
	port := freePort(t)
	m := newTestManager(port)
	m.unsetSystemProxy = func() error { return ErrUnsupportedDesktopEnvironment }

	if err := m.Set(1234, nil, allowAll); err != nil {
		t.Fatalf("first Set: %v", err)
	}

	err := m.Clear()
	if !errors.Is(err, ErrUnsupportedDesktopEnvironment) {
		t.Fatalf("Clear error = %v, want ErrUnsupportedDesktopEnvironment", err)
	}
	if m.server != nil || m.listener != nil {
		t.Fatal("server or listener not nil after Clear")
	}
	assertPortFree(t, port)

	// The regression: a second Set on the same fixed port must succeed.
	if err := m.Set(1234, nil, allowAll); err != nil {
		t.Fatalf("second Set on the same port: %v", err)
	}
	t.Cleanup(func() { m.Clear() })
	assertPACServed(t, port)
}

func TestSetOnOccupiedPortKeepsPreviousServer(t *testing.T) {
	port := freePort(t)
	m := newTestManager(port)
	if err := m.Set(1234, nil, allowAll); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	t.Cleanup(func() { m.Clear() })
	previous := m.server

	// The port is now held by the manager's own server.
	if err := m.Set(1234, nil, allowAll); err == nil {
		t.Fatal("second Set on the occupied port succeeded, want error")
	}
	if m.server != previous {
		t.Fatal("m.server was replaced by a failed Set")
	}
	assertPACServed(t, port)
}

func TestSetClosesServerOnSystemProxyFailure(t *testing.T) {
	port := freePort(t)
	m := newTestManager(port)
	m.setSystemProxy = func(string) error { return errors.New("boom") }
	unsetCalled := false
	m.unsetSystemProxy = func() error { unsetCalled = true; return nil }

	if err := m.Set(1234, nil, allowAll); err == nil {
		t.Fatal("Set succeeded, want error")
	}
	if m.server != nil || m.listener != nil {
		t.Fatal("server or listener not nil after failed Set")
	}
	if !unsetCalled {
		t.Fatal("unsetSystemProxy not called: partial system proxy writes were not rolled back")
	}
	assertPortFree(t, port)

	m.setSystemProxy = func(string) error { return nil }
	if err := m.Set(1234, nil, allowAll); err != nil {
		t.Fatalf("Set after failed Set: %v", err)
	}
	t.Cleanup(func() { m.Clear() })
}

func TestSetKeepsServerOnUnsupportedDE(t *testing.T) {
	port := freePort(t)
	m := newTestManager(port)
	m.setSystemProxy = func(string) error { return ErrUnsupportedDesktopEnvironment }
	unsetCalled := false
	m.unsetSystemProxy = func() error { unsetCalled = true; return nil }

	err := m.Set(1234, nil, allowAll)
	if !errors.Is(err, ErrUnsupportedDesktopEnvironment) {
		t.Fatalf("Set error = %v, want ErrUnsupportedDesktopEnvironment", err)
	}
	if unsetCalled {
		t.Fatal("unsetSystemProxy called on the unsupported-DE path")
	}
	t.Cleanup(func() { m.Clear() })
	if m.server == nil {
		t.Fatal("m.server nil: the PAC server must stay up for manual browser configuration")
	}
	assertPACServed(t, port)
}

func allowAll(string) bool { return true }

// newTestManager returns a Manager whose platform functions succeed without touching the OS.
func newTestManager(pacPort int) *Manager {
	m := NewManager(pacPort)
	m.setSystemProxy = func(string) error { return nil }
	m.unsetSystemProxy = func() error { return nil }
	return m
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// assertPortFree binds the port to prove nothing holds it. A failed Dial would
// not prove that: on macOS under load, Dial to an unbound port can succeed and
// only then be reset.
func assertPortFree(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("port %d still held: %v", port, err)
	}
	l.Close()
}

func assertPACServed(t *testing.T, port int) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/proxy.pac", port))
	if err != nil {
		t.Fatalf("GET proxy.pac: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET proxy.pac status = %d, want 200", resp.StatusCode)
	}
}

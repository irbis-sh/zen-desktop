// Package sysproxy implements [Manager], providing a unified, cross-platform interface for configuring system proxies.
//
// sysproxy uses PAC (Proxy Auto-Config) as the configuration method due to the extensive use of proxy exceptions.
// While declarative configuration methods also support exceptions, they often impose strict limits on the number
// of characters that can be specified. For example, the ProxyOverride registry key on Windows is limited to
// approximately 2000 characters, and the equivalent setting on macOS has a limit of around 650 characters.
// In contrast, PAC files can typically be up to 1MB in size, which is more than sufficient for our use case.
//
// To discover more about PAC, see:
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_PAC_file
package sysproxy

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/irbis-sh/process"
)

var ErrUnsupportedDesktopEnvironment = errors.New("system proxy configuration is currently only supported on GNOME and KDE")

type ShouldProxyFunc func(processPath string) bool

type Manager struct {
	pacPort int
	server  *http.Server
}

// NewManager creates a new system proxy Manager.
// The PAC server will listen on the given pacPort.
// If pacPort is 0, a random port will be chosen.
func NewManager(pacPort int) *Manager {
	return &Manager{
		pacPort: pacPort,
	}
}

// Set configures the system proxy to use the proxy server listening on the given port.
func (m *Manager) Set(proxyPort int, userConfiguredExcludedHosts []string, shouldProxy ShouldProxyFunc) error {
	if shouldProxy == nil {
		return fmt.Errorf("shouldProxy is nil")
	}

	pac := renderPac(proxyPort, userConfiguredExcludedHosts)

	actualPort, err := m.makeServer(pac, shouldProxy)
	if err != nil {
		return fmt.Errorf("make server: %v", err)
	}

	pacURL := fmt.Sprintf("http://127.0.0.1:%d/proxy.pac", actualPort)
	if err := setSystemProxy(pacURL); err != nil {
		return fmt.Errorf("set system proxy with URL %q: %w", pacURL, err)
	}

	return nil
}

// Clear removes the system proxy configuration.
func (m *Manager) Clear() error {
	if m.server == nil {
		log.Println("warning: trying to clear system proxy without setting it first")
		return nil
	}

	if err := unsetSystemProxy(); err != nil {
		return fmt.Errorf("unset system proxy: %w", err)
	}

	if err := m.server.Close(); err != nil {
		return fmt.Errorf("close: %v", err)
	}
	m.server = nil
	return nil
}

// makeServer starts an HTTP server that serves the PAC file.
// It returns the actual port the server is listening on, which may be different from the requested port if the latter is 0.
func (m *Manager) makeServer(pac []byte, shouldProxy ShouldProxyFunc) (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", func(w http.ResponseWriter, r *http.Request) {
		responsePAC := pac
		if !shouldProxy(processPathForRequest(r)) {
			responsePAC = transparentPAC
		}

		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.WriteHeader(http.StatusOK)
		w.Write(responsePAC)
	})

	m.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", m.pacPort))
	if err != nil {
		return -1, fmt.Errorf("listen: %v", err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	log.Printf("PAC server listening on port %d", actualPort)

	go func() {
		if err := m.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("error serving PAC: %v", err)
		}
	}()

	return actualPort, nil
}

func processPathForRequest(r *http.Request) string {
	pid, err := process.FindPIDByRequest(r)
	if err != nil {
		log.Printf("error finding PAC request process: %v", err)
		return ""
	}

	path, err := pid.ExecutablePath()
	if err != nil {
		log.Printf("error finding PAC request process path: %v", err)
		return ""
	}
	return path
}

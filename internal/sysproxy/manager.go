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

	"github.com/irbis-sh/zen-desktop/internal/process"
)

var ErrUnsupportedDesktopEnvironment = errors.New("system proxy configuration is currently only supported on GNOME and KDE")

type ShouldProxyFunc func(processPath string) bool

type Manager struct {
	pacPort  int
	server   *http.Server
	listener net.Listener

	// Platform functions, replaceable in tests.
	setSystemProxy   func(pacURL string) error
	unsetSystemProxy func() error
}

// NewManager creates a new system proxy Manager.
// The PAC server will listen on the given pacPort.
// If pacPort is 0, a random port will be chosen.
func NewManager(pacPort int) *Manager {
	return &Manager{
		pacPort:          pacPort,
		setSystemProxy:   setSystemProxy,
		unsetSystemProxy: unsetSystemProxy,
	}
}

// SetPACPort sets the port the PAC server will listen on the next time it is started.
// If pacPort is 0, a random port will be chosen. It has no effect on an already-running server.
func (m *Manager) SetPACPort(pacPort int) {
	m.pacPort = pacPort
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
	if err := m.setSystemProxy(pacURL); err != nil {
		err = fmt.Errorf("set system proxy with URL %q: %w", pacURL, err)
		if errors.Is(err, ErrUnsupportedDesktopEnvironment) {
			// The user points their browser at the PAC URL by hand, so leave the server up.
			return err
		}
		// Nobody will use the server, and the caller never reaches Clear on this path.
		// setSystemProxy writes per network service or per config key, so undo whatever
		// it managed to write before releasing the port.
		if unsetErr := m.unsetSystemProxy(); unsetErr != nil {
			log.Printf("warning: unset system proxy after failed set: %v", unsetErr)
		}
		if closeErr := m.closeServer(); closeErr != nil {
			log.Printf("warning: close PAC server after failed set: %v", closeErr)
		}
		return err
	}

	return nil
}

// Clear removes the system proxy configuration and stops the PAC server.
// The server is closed even if the system-level unset fails.
func (m *Manager) Clear() error {
	if m.server == nil {
		log.Println("warning: trying to clear system proxy without setting it first")
		return nil
	}

	closeErr := m.closeServer()
	unsetErr := m.unsetSystemProxy()
	return errors.Join(closeErr, unsetErr)
}

// closeServer stops the PAC server and guarantees its port is free on return,
// so a fixed PAC port can be bound again by the next Set.
// http.Server.Close only closes listeners that Serve has already registered, and
// Serve runs on its own goroutine, so closing the listener ourselves is what
// actually releases the port. The server goes first so that Serve, if it has not
// run yet, sees the shutdown and returns ErrServerClosed instead of logging an
// accept error. The listener's Close error is dropped on purpose: once Serve has
// registered it, Server.Close has already closed it.
func (m *Manager) closeServer() error {
	err := m.server.Close()
	m.listener.Close()
	m.server, m.listener = nil, nil
	return err
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

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", m.pacPort))
	if err != nil {
		return -1, fmt.Errorf("listen: %v", err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	log.Printf("PAC server listening on port %d", actualPort)

	// Assign the fields only once the listener is bound, so a failed bind does not
	// orphan a server from a previous Set.
	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	m.server, m.listener = server, listener

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("error serving PAC: %v", err)
		}
	}()

	return actualPort, nil
}

func processPathForRequest(r *http.Request) string {
	processInfo, err := process.FindByRequest(r)
	if err != nil {
		log.Printf("error finding PAC request process: %v", err)
	}

	return processInfo.ExecutablePath
}

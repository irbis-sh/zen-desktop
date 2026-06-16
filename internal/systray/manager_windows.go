package systray

import (
	"context"
	"log"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// OnProxyStarted should be called when the proxy gets started.
func (m *Manager) OnProxyStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyActive = true

	if m.startStopMenuItem == nil {
		// Sanity check.
		log.Println("startStopMenuItem is nil")
		return
	}

	m.startStopMenuItem.SetTitle("Stop")
	m.startStopMenuItem.SetTooltip("Stop")
}

// OnProxyStopped should be called when the proxy gets stopped.
func (m *Manager) OnProxyStopped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyActive = false

	if m.startStopMenuItem == nil {
		// Sanity check.
		log.Println("startStopMenuItem is nil")
		return
	}

	m.startStopMenuItem.SetTitle("Start")
	m.startStopMenuItem.SetTooltip("Start")
}

func (m *Manager) onReady(ctx context.Context) func() {
	return func() {
		setIcon(m.logoBytes)
		setTooltip(m.appName)

		openMenuItem := addMenuItem("Open", "Open the application window")
		go func() {
			for range openMenuItem.ClickedCh {
				runtime.Show(ctx)
			}
		}()

		m.startStopMenuItem = addMenuItem("Start", "Start")
		go func() {
			for range m.startStopMenuItem.ClickedCh {
				m.mu.Lock()
				active := m.proxyActive
				m.mu.Unlock()
				if active {
					m.proxyStop()
				} else {
					m.proxyStart()
				}
			}
		}()

		addSeparator()

		quitMenuItem := addMenuItem("Quit", "Quit the application")
		go func() {
			for range quitMenuItem.ClickedCh {
				runtime.Quit(ctx)
			}
		}()
	}
}

package systray

import (
	"context"
	"log"
	"sync/atomic"

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
	m.startStopMenuItem.update()
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
	m.startStopMenuItem.update()
}

func (m *Manager) onReady(ctx context.Context) func() {
	return func() {
		setIcon(m.logoBytes)

		openMenuItem := addMenuItem("Open")
		go func() {
			for range openMenuItem.ClickedCh {
				runtime.Show(ctx)
			}
		}()

		m.mu.Lock()
		active := m.proxyActive
		m.mu.Unlock()

		if active {
			m.startStopMenuItem = addMenuItem("Stop")
		} else {
			m.startStopMenuItem = addMenuItem("Start")
		}

		go func() {
			for range m.startStopMenuItem.ClickedCh {
				m.mu.Lock()
				active = m.proxyActive
				m.mu.Unlock()
				if active {
					m.proxyStop()
				} else {
					m.proxyStart()
				}
			}
		}()

		id := atomic.AddUint32(&currentID, 1)
		addSeparator(id)

		quitMenuItem := addMenuItem("Quit")
		go func() {
			for range quitMenuItem.ClickedCh {
				runtime.Quit(ctx)
			}
		}()
	}
}

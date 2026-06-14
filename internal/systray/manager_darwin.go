package systray

// To be implemented.

import (
	"context"
)

type Manager struct{}

func NewManager(string, func(), func()) (*Manager, error) {
	return &Manager{}, nil
}

func (m *Manager) Init(context.Context) error {
	return nil
}

func (m *Manager) Quit() {}

// OnProxyStarted should be called when the proxy gets started.
func (m *Manager) OnProxyStarted() {}

// OnProxyStopped should be called when the proxy gets stopped.
func (m *Manager) OnProxyStopped() {}

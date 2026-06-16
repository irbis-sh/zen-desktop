//go:build !darwin

package systray

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sync"
)

//go:embed logo.ico
var logoFS embed.FS

type Manager struct {
	logoBytes         []byte
	appName           string
	mu                sync.Mutex
	proxyActive       bool
	proxyStart        func()
	proxyStop         func()
	startStopMenuItem *menuItem
}

func NewManager(appName string, proxyStart func(), proxyStop func()) (*Manager, error) {
	if appName == "" {
		return nil, errors.New("appName is empty")
	}
	if proxyStart == nil {
		return nil, errors.New("proxyStart is nil")
	}
	if proxyStop == nil {
		return nil, errors.New("proxyStop is nil")
	}

	logoBytes, err := logoFS.ReadFile("logo.ico")
	if err != nil {
		return nil, fmt.Errorf("read logo from embed: %w", err)
	}

	return &Manager{
		logoBytes:  logoBytes,
		proxyStart: proxyStart,
		proxyStop:  proxyStop,
		appName:    appName,
	}, nil
}

func (m *Manager) Init(ctx context.Context) error {
	go func() {
		run(m.onReady(ctx), nil)
	}()

	return nil
}

// Quit needs to be called on application quit.
func (m *Manager) Quit() {
	quit()
}

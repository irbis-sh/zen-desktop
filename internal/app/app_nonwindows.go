//go:build !windows

package app

import (
	"context"

	"github.com/godbus/dbus/v5"
)

func (a *App) Startup(ctx context.Context) {
	a.commonStartup(ctx)
}
func (a *App) TrayAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}

	obj := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher")
	v, err := obj.GetProperty("org.kde.StatusNotifierWatcher.IsStatusNotifierHostRegistered")
	if err != nil {
		return false
	}
	registered, ok := v.Value().(bool)
	return registered && ok
}

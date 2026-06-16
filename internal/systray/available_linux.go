package systray

import "github.com/godbus/dbus/v5"

func Available() bool {
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

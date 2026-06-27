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
	// Require both a registered StatusNotifier host AND the AppIndicator library
	// to be loadable. Otherwise, on a host with a tray (e.g. KDE) but without the
	// client library, HideWindowOnClose would hide the window into a tray that
	// has no icon and no way to reopen it.
	return registered && ok && trayLibraryAvailable()
}

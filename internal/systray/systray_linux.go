//go:build linux
// +build linux

package systray

/*
	This file contains code from the systray project (https://github.com/getlantern/systray), licensed under the Apache License.
	See more in the COPYING.md file in the root directory of this project.
*/

// #cgo linux pkg-config: gtk+-3.0
// #cgo linux LDFLAGS: -ldl
// #include "systray.h"
import "C"

import (
	"unsafe"
)

func registerSystray() {
	C.registerSystray()
}

func nativeLoop() {
	C.nativeLoop()
}

func quit() {
	C.quit()
}

// trayLibraryAvailable reports whether the AppIndicator library could be loaded
// at runtime. It only performs dlopen/dlsym (no GTK), so it is safe to call
// before GTK is initialized. The underlying C cache is not synchronized for
// concurrent first-time calls, so it must be primed once on a single goroutine
// before the GTK loop starts; in practice Available() does this during
// wails.Run setup. See try_load_appindicator in systray_linux.c.
func trayLibraryAvailable() bool {
	return C.try_load_appindicator() != 0
}

// addMenuItem adds a menu item with the designated title for Linux.
// It can be safely invoked from different goroutines.
// On Windows and OSX this is the same as calling addMenuItem
//
// NOTE: tooltip is not set on Linux.
func addMenuItem(title string) *menuItem {
	item := newMenuItem(title, "", nil)
	item.update()
	return item
}

// setIcon sets the systray icon.
// iconBytes should be the content of .ico for windows and .ico/.jpg/.png
// for other platforms.
func setIcon(iconBytes []byte) {
	cstr := (*C.char)(unsafe.Pointer(&iconBytes[0]))
	C.setIcon(cstr, (C.int)(len(iconBytes)), false)
}

// SetTitle sets the systray title, only available on Mac and Linux.
func (item *menuItem) SetTitle(title string) {
	item.title = title
	item.update()
}

func addOrUpdateMenuItem(item *menuItem) {
	var disabled C.short
	if item.disabled {
		disabled = 1
	}
	var checked C.short
	if item.checked {
		checked = 1
	}
	var isCheckable C.short
	if item.isCheckable {
		isCheckable = 1
	}
	var parentID uint32
	if item.parent != nil {
		parentID = item.parent.id
	}
	C.add_or_update_menu_item(
		C.int(item.id),
		C.int(parentID),
		C.CString(item.title),
		C.CString(item.tooltip),
		disabled,
		checked,
		isCheckable,
	)
}

func addSeparator(id uint32) {
	C.add_separator(C.int(id))
}

func hideMenuItem(item *menuItem) {
	C.hide_menu_item(
		C.int(item.id),
	)
}

func showMenuItem(item *menuItem) {
	C.show_menu_item(
		C.int(item.id),
	)
}

//export systray_ready
func systray_ready() {
	systrayReady()
}

//export systray_on_exit
func systray_on_exit() {
	systrayExit()
}

//export systray_menu_item_selected
func systray_menu_item_selected(cID C.int) {
	systrayMenuItemSelected(uint32(cID))
}

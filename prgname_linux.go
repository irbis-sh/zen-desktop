package main

/*
#cgo pkg-config: glib-2.0
#include <glib.h>
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"github.com/irbis-sh/zen-desktop/internal/constants"
)

// WebKit names its data folders after GLib's program name, which defaults to
// the binary's file name. That differs between packages: zen from install.sh
// and Nix, zen-adblocker from the AUR, Zen from the release archive. Wails'
// ProgramName option sets the name only after creating the webview, which is
// too late.
func init() {
	name := C.CString(constants.AppNameLowercase)
	defer C.free(unsafe.Pointer(name))
	C.g_set_prgname(name)
}

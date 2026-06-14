package systray

/*
	This file contains code from the systray project (https://github.com/getlantern/systray), licensed under the Apache License.
	See more in the COPYING.md file in the root directory of this project.
*/

import (
	"log"
	"sync"
	"sync/atomic"
)

var quitOnce sync.Once

// setTooltip sets the systray tooltip to display on mouse hover of the tray icon.
func setTooltip(tooltip string) {
	if err := wt.setTooltip(tooltip); err != nil {
		log.Printf("Unable to set tooltip: %v", err)
		return
	}
}

// setIcon sets the systray icon.
// iconBytes should be the content of a .ico file.
func setIcon(iconBytes []byte) {
	iconFilePath, err := iconBytesToFilePath(iconBytes)
	if err != nil {
		log.Printf("Unable to write icon data to temp file: %v", err)
		return
	}
	if err := wt.setIcon(iconFilePath); err != nil {
		log.Printf("Unable to set icon: %v", err)
		return
	}
}

// quit the systray. This can be called from any goroutine.
func quit() {
	quitOnce.Do(quitInternal)
}

// addMenuItem adds a menu item with the designated title and tooltip.
// It can be safely invoked from different goroutines.
// Created menu items are checkable on Windows and OSX by default. For Linux you have to use addMenuItem.
func addMenuItem(title string, tooltip string) *menuItem {
	item := newMenuItem(title, tooltip, nil)
	item.update()
	return item
}

// addSeparator adds a separator bar to the menu.
func addSeparator() {
	id := atomic.AddUint32(&currentID, 1)
	err := wt.addSeparatorMenuItem(id, 0)
	if err != nil {
		log.Printf("Unable to addSeparator: %v", err)
		return
	}
}

// AddSubMenuItem adds a nested sub-menu item with the designated title and tooltip.
// It can be safely invoked from different goroutines.
// Created menu items are checkable on Windows and OSX by default. For Linux you have to use AddSubMenuItemCheckbox.
func (item *menuItem) AddSubMenuItem(title string, tooltip string) *menuItem {
	child := newMenuItem(title, tooltip, item)
	child.update()
	return child
}

// AddSubMenuItemCheckbox adds a nested sub-menu item with the designated title and tooltip and a checkbox for Linux.
// It can be safely invoked from different goroutines.
// On Windows and OSX this is the same as calling AddSubMenuItem.
func (item *menuItem) AddSubMenuItemCheckbox(title string, tooltip string, checked bool) *menuItem {
	child := newMenuItem(title, tooltip, item)
	child.isCheckable = true
	child.checked = checked
	child.update()
	return child
}

// SetTitle set the text to display on a menu item.
func (item *menuItem) SetTitle(title string) {
	item.title = title
	item.update()
}

// SetTooltip set the tooltip to show when mouse hover.
func (item *menuItem) SetTooltip(tooltip string) {
	item.tooltip = tooltip
	item.update()
}

// Disabled checks if the menu item is disabled.
func (item *menuItem) Disabled() bool {
	return item.disabled
}

// Enable a menu item regardless if it's previously enabled or not.
func (item *menuItem) Enable() {
	item.disabled = false
	item.update()
}

// Disable a menu item regardless if it's previously disabled or not.
func (item *menuItem) Disable() {
	item.disabled = true
	item.update()
}

// Hide hides a menu item.
func (item *menuItem) Hide() {
	hideMenuItem(item)
}

// Show shows a previously hidden menu item.
func (item *menuItem) Show() {
	showMenuItem(item)
}

// Checked returns if the menu item has a check mark.
func (item *menuItem) Checked() bool {
	return item.checked
}

// Check a menu item regardless if it's previously checked or not.
func (item *menuItem) Check() {
	item.checked = true
	item.update()
}

// Uncheck a menu item regardless if it's previously unchecked or not.
func (item *menuItem) Uncheck() {
	item.checked = false
	item.update()
}

//go:build !darwin

package systray

/*
	This file contains code from the systray project (https://github.com/getlantern/systray), licensed under the Apache License.
	See more in the COPYING.md file in the root directory of this project.
*/

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	systrayReady  func()
	systrayExit   func()
	menuItems     = make(map[uint32]*menuItem)
	menuItemsLock sync.RWMutex

	currentID = uint32(0)
)

// menuItem is used to keep track each menu item of systray.
// Don't create it directly, use the one systray.AddMenuItem() returned.
type menuItem struct {
	// ClickedCh is the channel which will be notified when the menu item is clicked
	ClickedCh chan struct{}

	// id uniquely identify a menu item, not supposed to be modified
	id uint32
	// title is the text shown on menu item
	title string
	// tooltip is the text shown when pointing to menu item
	tooltip string
	// disabled menu item is grayed out and has no effect when clicked
	disabled bool
	// checked menu item has a tick before the title
	checked bool
	// has the menu item a checkbox (Linux)
	isCheckable bool
	// parent item, for sub menus
	parent *menuItem
}

func (item *menuItem) String() string {
	if item.parent == nil {
		return fmt.Sprintf("MenuItem[%d, %q]", item.id, item.title)
	}
	return fmt.Sprintf("MenuItem[%d, parent %d, %q]", item.id, item.parent.id, item.title)
}

// newMenuItem returns a populated MenuItem object.
func newMenuItem(title string, tooltip string, parent *menuItem) *menuItem {
	return &menuItem{
		ClickedCh:   make(chan struct{}),
		id:          atomic.AddUint32(&currentID, 1),
		title:       title,
		tooltip:     tooltip,
		disabled:    false,
		checked:     false,
		isCheckable: false,
		parent:      parent,
	}
}

// run initializes GUI and starts the event loop, then invokes the onReady callback. It blocks until
// systray.Quit() is called. It must be run from the main thread on macOS.
func run(onReady func(), onExit func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if onReady == nil {
		systrayReady = func() {}
	} else {
		// Run onReady on separate goroutine to avoid blocking event loop
		readyCh := make(chan interface{})
		go func() {
			<-readyCh
			onReady()
		}()
		systrayReady = func() {
			close(readyCh)
		}
	}
	// unlike onReady, onExit runs in the event loop to make sure it has time to
	// finish before the process terminates
	if onExit == nil {
		onExit = func() {}
	}
	systrayExit = onExit
	registerSystray()
	nativeLoop()
}

func systrayMenuItemSelected(id uint32) {
	menuItemsLock.RLock()
	item, ok := menuItems[id]
	menuItemsLock.RUnlock()
	if !ok {
		log.Printf("no menu item with ID %v", id)
		return
	}
	select {
	case item.ClickedCh <- struct{}{}:
	// in case no one waiting for the channel
	default:
	}
}

// update propagates changes on a menu item to systray.
func (item *menuItem) update() {
	menuItemsLock.Lock()
	menuItems[item.id] = item
	menuItemsLock.Unlock()
	addOrUpdateMenuItem(item)
}

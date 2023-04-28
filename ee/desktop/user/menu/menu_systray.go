package menu

import (
	"sync"

	"github.com/kolide/systray"
)

var (
	buildMutex sync.Mutex
	doneChans  []chan<- struct{}
	// We rely on systray to notify us if it's appearance is dark mode or not
	// systrayDarkMode caches this flag so we build the menu properly
	systrayDarkMode bool
	// systrayMenuIcon caches the icon type, so that we can re-set the icon when changing between dark & light modes
	systrayMenuIcon menuIcon
)

// Init creates the menu bar & icon. It must be called on the main thread, and
// blocks until Shutdown() is called.
func (m *menu) Init() {
	// Build will be invoked after the menu has been initialized
	// Before the menu exits, cleanup the goroutines
	systray.Run(m.Build, m.cleanup, m.onAppearanceChanged)
}

// onAppearanceChanged is called by systray when the menu bar's effective appearance changes between dark and light
// In practice, this is only used on macOS
func (m *menu) onAppearanceChanged(dark bool) {
	systrayDarkMode = dark
	// For some reason, macOS will send multiple notifications of effectiveAppearance changes, alternating back and forth between dark and light
	// Since dark/light mode only affects the icon, just set the icon here instead of rebuilding the menu entirely
	m.setIcon(systrayMenuIcon)
}

// Build parses the menu file and constructs the menu. If a menu already exists,
// all of its items will be removed before the new menu is built.
func (m *menu) Build() {
	// Lock so the menu is never being modified by more than one goroutine at a time
	buildMutex.Lock()
	defer buildMutex.Unlock()

	// Remove all menu items each time we rebuild the menu
	systray.ResetMenu()

	// Even though the menu items have been removed, we still have goroutines hanging around
	m.cleanup()

	// Reparse the menu file & rebuild the menu
	menuData := m.getMenuData()
	parseMenuData(menuData, m)
}

func (m *menu) setIcon(icon menuIcon) {
	systrayMenuIcon = icon
	iconBytes := getIcon(icon)
	if iconBytes != nil {
		systray.SetTemplateIcon(iconBytes, iconBytes)
	}
}

func (m *menu) setTooltip(tooltip string) {
	systray.SetTooltip(tooltip)
}

func (m *menu) addMenuItem(label, tooltip string, disabled bool, ap ActionPerformer, parent any) any {
	if label == "" {
		return nil
	}

	var item, parentItem *systray.MenuItem
	parentItem, ok := parent.(*systray.MenuItem)
	if ok {
		// If a parent menu item was provided, this is meant to be a sub menu item
		item = parentItem.AddSubMenuItem(label, tooltip)
	} else {
		item = systray.AddMenuItem(label, tooltip)
	}

	if disabled {
		item.Disable()
	}

	// Setup a handler to perform the menu item's action
	m.makeActionHandler(item, ap)

	return item
}

func (m *menu) addSeparator() {
	systray.AddSeparator()
}

// Shutdown quits the menu. It unblocks the Init() call.
func (m *menu) Shutdown() {
	systray.Quit()
}

// Cleans up goroutines associated with menu items
func (m *menu) cleanup() {
	for _, done := range doneChans {
		close(done)
	}
	doneChans = nil
}

// Returns true if launcher is running in production
func (m *menu) isProd() bool {
	return m.hostname == "k2device-preprod.kolide.com" || m.hostname == "k2device.kolide.com"
}

// makeActionHandler creates a handler to execute the desired action when a menu item is clicked
func (m *menu) makeActionHandler(item *systray.MenuItem, ap ActionPerformer) {
	if ap == nil {
		// No action to handle
		return
	}

	// Create and hold on to a done channel for each action, so we don't leak goroutines
	done := make(chan struct{})
	doneChans = append(doneChans, done)

	go func() {
		for {
			select {
			case <-item.ClickedCh:
				// Menu item was clicked
				ap.Perform(m)
			case <-done:
				// Menu item is going away
				return
			}
		}
	}()
}

package menu

import (
	"encoding/json"
	"os"
	"sync"

	"fyne.io/systray"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/assets"
)

// MenuData encapsulates a menu bar icon and accessible menu items
type MenuData struct {
	Tooltip string         `json:"tooltip,omitempty"`
	Items   []menuItemData `json:"items"`
}

// menuItemData represents a menu item, optionally containing sub menu items
type menuItemData struct {
	Label       string         `json:"label,omitempty"`
	Tooltip     string         `json:"tooltip,omitempty"`
	Disabled    bool           `json:"disabled,omitempty"`
	NonProdOnly bool           `json:"nonProdOnly,omitempty"`
	IsSeparator bool           `json:"isSeparator,omitempty"`
	Action      actionData     `json:"action,omitempty"`
	Items       []menuItemData `json:"items,omitempty"`
}

// MenuBuilder is an interface a menu parser can use to specify how the menu is built
type MenuBuilder interface {
	// SetIcon sets the menu bar icon
	SetIcon()
	// SetTooltip sets the menu tooltip
	SetTooltip(tooltip string)
	// AddMenuItem creates a menu item with the supplied attributes. If the menu item is successfully
	// created, it is returned. If parent is non-nil, the menu item will be created as a child of parent.
	AddMenuItem(label, tooltip string, disabled, nonProdOnly bool, action Action, parent any) any
	// AddSeparator adds a separator to the menu
	AddSeparator()
}

type menu struct {
	logger     log.Logger
	hostname   string
	filePath   string
	buildMutex sync.Mutex
	doneChans  []chan<- struct{}
}

func New(logger log.Logger, hostname, filePath string) *menu {
	m := &menu{
		logger:   logger,
		hostname: hostname,
		filePath: filePath,
	}

	return m
}

// Init creates the menu bar & icon. It must be called on the main thread, and
// blocks until Shutdown() is called.
func (m *menu) Init() {
	// Build will be invoked after the menu has been initialized
	// Before the menu exits, cleanup the goroutines
	systray.Run(m.Build, m.cleanup)
}

// Build parses the menu file and constructs the menu. If a menu already exists,
// all of its items will be removed before the new menu is built.
func (m *menu) Build() {
	// Lock so the menu is never being modified by more than one goroutine at a time
	m.buildMutex.Lock()
	defer m.buildMutex.Unlock()

	// Remove all menu items each time we rebuild the menu
	systray.ResetMenu()

	// Even though the menu items have been removed, we still have goroutines hanging around
	m.cleanup()

	// Reparse the menu file & rebuild the menu
	menuData := m.getMenuData()
	if menuData == nil {
		var defaultMenuData MenuData
		menuData = &defaultMenuData
	}
	parseMenuData(menuData, m)
}

func (m *menu) getMenuData() *MenuData {
	statusFileBytes, err := os.ReadFile(m.filePath)
	if err != nil {
		level.Error(m.logger).Log("msg", "failed to read menu file", "path", m.filePath)
		return nil
	}

	var menu MenuData
	if err := json.Unmarshal(statusFileBytes, &menu); err != nil {
		level.Error(m.logger).Log("msg", "failed to unmarshal menu json")
		return nil
	}

	return &menu
}

func (m *menu) SetIcon() {
	// For now, icons are hard-coded
	if m.isProd() {
		systray.SetTemplateIcon(assets.KolideDesktopIcon, assets.KolideDesktopIcon)
	} else {
		systray.SetTemplateIcon(assets.KolideDebugDesktopIcon, assets.KolideDebugDesktopIcon)
	}
}

func (m *menu) SetTooltip(tooltip string) {
	systray.SetTooltip(tooltip)
}

func (m *menu) AddMenuItem(label, tooltip string, disabled, nonProdOnly bool, action Action, parent any) any {
	if nonProdOnly && m.isProd() {
		// This is prod environment, but the menu item is for non-prod only
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
	m.makeActionHandler(item, action)

	return item
}

func (m *menu) AddSeparator() {
	systray.AddSeparator()
}

// Shutdown quits the menu. It unblocks the Init() call.
func (m *menu) Shutdown() {
	systray.Quit()
}

// Cleans up goroutines associated with menu items
func (m *menu) cleanup() {
	for _, done := range m.doneChans {
		close(done)
	}
	m.doneChans = nil
}

// Returns true if launcher is running in production
func (m *menu) isProd() bool {
	return m.hostname == "k2device-preprod.kolide.com" || m.hostname == "k2device.kolide.com"
}

// makeActionHandler creates a handler to execute the desired action when a menu item is clicked
func (m *menu) makeActionHandler(item *systray.MenuItem, action Action) {
	// Create and hold on to a done channel for each action, so we don't leak goroutines
	done := make(chan struct{})
	m.doneChans = append(m.doneChans, done)

	go func() {
		for {
			select {
			case <-item.ClickedCh:
				// Menu item was clicked
				action.Perform(m)
			case <-done:
				// Menu item is going away
				return
			}
		}
	}()
}

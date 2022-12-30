package menu

import (
	"os/exec"
	"runtime"

	"fyne.io/systray"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/assets"
)

// MenuData encapsulates a systray icon and accessible menu items
type MenuData struct {
	Tooltip string         `json:"tooltip,omitempty"`
	Items   []MenuItemData `json:"items"`
}

// MenuItemData represents a menu item, optionally containing sub menu items
type MenuItemData struct {
	Label       string         `json:"label,omitempty"`
	Tooltip     string         `json:"tooltip,omitempty"`
	Disabled    bool           `json:"disabled,omitempty"`
	NonProdOnly bool           `json:"nonProdOnly,omitempty"`
	IsSeparator bool           `json:"isSeparator,omitempty"`
	Action      MenuItemAction `json:"action,omitempty"`
	Items       []MenuItemData `json:"items,omitempty"`
}

// MenuItemAction encapsulates what action should be performed when a menu item is invoked
type MenuItemAction struct {
	URL string `json:"url,omitempty"`
}

// MenuBuilder is an interface a menu parser can use to specify how the systray menu is built
type MenuBuilder interface {
	SetTooltip(tooltip string)
	AddMenuItem(label, tooltip string, disabled, nonProdOnly bool, action MenuItemAction, parent any) any
	AddSeparator()
}

type menu struct {
	logger   log.Logger
	hostname string
}

func New(logger log.Logger, hostname string) *menu {
	m := &menu{
		logger:   logger,
		hostname: hostname,
	}

	return m
}

func (m *menu) Init(buildMenu func()) {
	systray.Run(buildMenu, nil)
}

func (m *menu) Build(menu *MenuData) {
	// Remove all menu items each time we rebuild the menu
	systray.ResetMenu()

	parseMenuData(menu, m)
}

func (m *menu) SetTooltip(tooltip string) {
	systray.SetTooltip(tooltip)

	if m.isProd() {
		systray.SetTemplateIcon(assets.KolideDesktopIcon, assets.KolideDesktopIcon)
	} else {
		systray.SetTemplateIcon(assets.KolideDebugDesktopIcon, assets.KolideDebugDesktopIcon)
	}
}

func (m *menu) AddMenuItem(label, tooltip string, disabled, nonProdOnly bool, action MenuItemAction, parent any) any {
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

	// Menu items can have actions associated with them
	m.addAction(item, action)

	return item
}

func (m *menu) AddSeparator() {
	systray.AddSeparator()
}

func (m *menu) Shutdown() {
	systray.Quit()
}

func (m *menu) isProd() bool {
	return m.hostname == "k2device-preprod.kolide.com" || m.hostname == "k2device.kolide.com"
}

func (m *menu) addAction(item *systray.MenuItem, action MenuItemAction) {
	if action.URL == "" {
		return
	}

	go func() {
		for {
			select {
			case <-item.ClickedCh:
				err := open(action.URL)
				if err != nil {
					level.Error(m.logger).Log("msg", "failed to open URL in browser", "err", err)
				}
			}
		}
	}()
}

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "/usr/bin/open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

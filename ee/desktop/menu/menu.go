package menu

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/version"
)

// menuIcons are named identifiers
type menuIcon string

const (
	KolideDesktopIcon      = "kolide-desktop"
	KolideDebugDesktopIcon = "kolide-debug-desktop"
)

// MenuData encapsulates a menu bar icon and accessible menu items
type MenuData struct {
	Icon    menuIcon       `json:"icon"`
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
	Action      Action         `json:"action,omitempty"`
	Items       []menuItemData `json:"items,omitempty"`
}

// MenuBuilder is an interface a menu parser can use to specify how the menu is built
type MenuBuilder interface {
	// SetIcon sets the menu bar icon
	SetIcon(icon menuIcon)
	// SetTooltip sets the menu tooltip
	SetTooltip(tooltip string)
	// AddMenuItem creates a menu item with the supplied attributes. If the menu item is successfully
	// created, it is returned. If parent is non-nil, the menu item will be created as a child of parent.
	AddMenuItem(label, tooltip string, disabled, nonProdOnly bool, ap ActionPerformer, parent any) any
	// AddSeparator adds a separator to the menu
	AddSeparator()
}

// menu handles common functionality like retrieving menu data, and allows menu builders to provide their implementations
type menu struct {
	logger   log.Logger
	hostname string
	filePath string
}

func New(logger log.Logger, hostname, filePath string) *menu {
	m := &menu{
		logger:   logger,
		hostname: hostname,
		filePath: filePath,
	}

	return m
}

// getMenuData ingests the shared menu.json file created by the desktop runner
// It unmarshals the data into a MenuData struct representing the menu, which is suitable for parsing and building the menu
// Here is an example of valid JSON
/*
  	{
		"icon": "kolide-desktop",
		"tooltip": "Kolide",
		"items": [
		{
			"label": "Kolide Agent is running",
			"disabled": true
		},
		{
			"isSeparator": true
		},
		{
			"label": "Failing checks",
			"items": [
			{
				"label": "Ensure Kolide Agent Has Full Disk Access Entitlement",
				"action": {
				"type": "open-url",
				"action": {
					"url": "https://help.kolide.com/en/articles/3387759-how-to-grant-macos-full-disk-access-to-kolide"
				}
				}
			},
			]
		}
		]
  	}
*/
func (m *menu) getMenuData() *MenuData {
	if m.filePath == "" {
		return nil
	}

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

// getDefaultMenu returns a static, hard-coded menu to be used in failure modes when the menu data can't be retrieved
func getDefaultMenu() *MenuData {
	data := &MenuData{
		Icon:    KolideDesktopIcon,
		Tooltip: "Kolide",
		Items: []menuItemData{
			{
				Label:    fmt.Sprintf("Version %s", version.Version().Version),
				Disabled: true,
			},
		},
	}

	return data
}

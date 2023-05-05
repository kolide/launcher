package menu

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/version"
)

//go:embed initial_menu.json
var InitialMenu []byte

// menuIcons are named identifiers
type menuIcon string

const (
	TranslucentIcon         menuIcon = "translucent"
	DefaultIcon             menuIcon = "default"
	TriangleExclamationIcon menuIcon = "triangle-exclamation"
	CircleXIcon             menuIcon = "circle-x"
)

// MenuData encapsulates a menu bar icon and accessible menu items
type MenuData struct {
	Icon    menuIcon       `json:"icon"`
	Tooltip string         `json:"tooltip,omitempty"`
	Items   []menuItemData `json:"items"`
}

// menuItemData represents a menu item, optionally containing sub menu items
type menuItemData struct {
	Label     string         `json:"label,omitempty"`
	Tooltip   string         `json:"tooltip,omitempty"`
	Disabled  bool           `json:"disabled,omitempty"` // Whether the item is grey text, or selectable
	Separator bool           `json:"separator,omitempty"`
	Action    Action         `json:"action,omitempty"`
	Items     []menuItemData `json:"items,omitempty"`
}

// menuBuilder is an interface a menu parser can use to specify how the menu is built
type menuBuilder interface {
	// setIcon sets the menu bar icon
	setIcon(icon menuIcon)
	// setTooltip sets the menu tooltip
	setTooltip(tooltip string)
	// addMenuItem creates a menu item with the supplied attributes. If the menu item is successfully
	// created, it is returned. If parent is non-nil, the menu item will be created as a child of parent.
	addMenuItem(label, tooltip string, disabled bool, ap ActionPerformer, parent any) any
	// addSeparator adds a separator to the menu
	addSeparator()
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
func (m *menu) getMenuData() *MenuData {
	// Ensure that at a minimum we return a default menu, in case reading/unmarshaling fails
	var menu MenuData
	defer menu.SetDefaults()

	if m.filePath == "" {
		return &menu
	}

	menuFileBytes, err := os.ReadFile(m.filePath)
	if err != nil {
		level.Error(m.logger).Log("msg", "failed to read menu file", "path", m.filePath)
		return &menu
	}

	if err := json.Unmarshal(menuFileBytes, &menu); err != nil {
		level.Error(m.logger).Log("msg", "failed to unmarshal menu json")
		return &menu
	}

	return &menu
}

// SetDefaults ensures we have the desired default values.
func (md *MenuData) SetDefaults() {
	if md.Icon == "" {
		md.Icon = DefaultIcon
	}

	if md.Tooltip == "" {
		md.Tooltip = "Kolide"
	}

	// It should be unheard of to have a menu with no items, but just in case...
	if md.Items == nil {
		md.Items = []menuItemData{
			{
				Label:    fmt.Sprintf("Kolide Agent Version %s", version.Version().Version),
				Disabled: true,
			},
		}

	}
}

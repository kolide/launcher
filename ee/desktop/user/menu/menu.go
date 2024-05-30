package menu

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

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
	CircleDotIcon           menuIcon = "circle-dot"
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
	slogger  *slog.Logger
	hostname string
	filePath string
	urlInput chan string
}

func New(slogger *slog.Logger, hostname, filePath string, urlInput chan string) *menu {
	m := &menu{
		slogger:  slogger.With("component", "desktop_menu"),
		hostname: hostname,
		filePath: filePath,
		urlInput: urlInput,
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
		m.slogger.Log(context.TODO(), slog.LevelError,
			"failed to read menu file",
			"path", m.filePath,
			"err", err,
		)
		return &menu
	}

	if err := json.Unmarshal(menuFileBytes, &menu); err != nil {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"failed to unmarshal menu json",
			"err", err,
		)
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

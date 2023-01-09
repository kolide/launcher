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

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
var initialMenu []byte

// menuIcons are named identifiers
type menuIcon string

const (
	KolideDesktopIcon = "kolide-desktop"
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
	Disabled    bool           `json:"disabled,omitempty"` // Whether the item is grey text, or selectable
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
func (m *menu) getMenuData() *MenuData {
	if m.filePath == "" {
		return nil
	}

	fileBytes, err := os.ReadFile(m.filePath)
	if err != nil {
		level.Error(m.logger).Log("msg", "failed to read menu file", "path", m.filePath)
		return nil
	}

	md, err := newMenuDataFromBytes(fileBytes)
	if err != nil {
		level.Error(m.logger).Log("msg", "failed to parse menu file", "error", err)
		return nil
	}

	return md
}

func newMenuDataFromBytes(menuBytes []byte) (*MenuData, error) {
	var md MenuData
	if err := json.Unmarshal(menuBytes, &md); err != nil {
		return nil, fmt.Errorf("failed to unmarshal menu json: %w", err)
	}

	md.SetDefaults()

	return &md, nil
}

func newInitialMenuData() (*MenuData, error) {
	md, err := newMenuDataFromBytes(initialMenu)
	if err != nil {
		return nil, fmt.Errorf("initial menu setup: %w", err)
	}

	return md, nil
}

// SetDefaults ensures we have the desired default values.
func (md *MenuData) SetDefaults() {
	if md.Icon == "" {
		md.Icon = KolideDesktopIcon
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

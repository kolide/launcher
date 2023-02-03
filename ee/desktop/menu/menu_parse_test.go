package menu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testMenuBuilder struct {
	Icon     menuIcon
	Tooltip  string
	parent   any
	itemCopy *menuItemData
	menuCopy MenuData
}

func (m *testMenuBuilder) SetIcon(icon menuIcon) {
	m.menuCopy.Icon = icon
}

func (m *testMenuBuilder) SetTooltip(tooltip string) {
	m.menuCopy.Tooltip = tooltip
}

func (m *testMenuBuilder) AddMenuItem(label, tooltip string, disabled, nonProdOnly bool, ap ActionPerformer, parent any) any {
	item := &menuItemData{
		Label:       label,
		Tooltip:     tooltip,
		Disabled:    disabled,
		NonProdOnly: nonProdOnly,
	}

	if parent != nil {
		m.itemCopy.Items = append(m.itemCopy.Items, *item)
		return m.parent
	} else {
		m.itemCopy = item
	}

	return m.parent
}

func (m *testMenuBuilder) AddSeparator() {
	m.itemCopy = &menuItemData{
		IsSeparator: true,
	}
}

func Test_ParseMenuData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data *MenuData
	}{
		{
			name: "nil",
		},
		{
			name: "default",
			data: &MenuData{},
		},
		{
			name: "happy path",
			data: &MenuData{
				Icon:    KolideDebugDesktopIcon,
				Tooltip: "Kolide",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := &testMenuBuilder{}
			parseMenuData(tt.data, builder)
			if tt.data != nil {
				assert.Equal(t, *tt.data, builder.menuCopy)
			}
		})
	}
}

func Test_ParseMenuItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data *menuItemData
	}{
		{
			name: "nil",
		},
		{
			name: "default",
			data: &menuItemData{Label: "something"},
		},
		{
			name: "one item",
			data: &menuItemData{Label: "first item"},
		},
		{
			name: "non prod",
			data: &menuItemData{Label: "non prod item", NonProdOnly: true},
		},
		{
			name: "separator",
			data: &menuItemData{IsSeparator: true},
		},
		{
			name: "submenu",
			data: &menuItemData{
				Label: "parent",
				Items: []menuItemData{
					{Label: "first item"},
					{Label: "second item"},
				}},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := &testMenuBuilder{parent: menuItemData{Label: "parent item"}}
			parseMenuItem(tt.data, builder, nil)
			if tt.data == nil {
				assert.Equal(t, tt.data, builder.itemCopy)
			} else {
				assert.Equal(t, *tt.data, *builder.itemCopy)
			}
		})
	}
}

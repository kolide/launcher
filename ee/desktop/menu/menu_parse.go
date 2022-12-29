package menu

func parseMenuData(m *MenuData, builder MenuBuilder) {
	// Set top-level systray properties
	builder.SetTooltip(m.Tooltip)

	for _, child := range m.Items {
		parseMenuItem(&child, builder, nil)
	}
}

func parseMenuItem(m *MenuItemData, builder MenuBuilder, parent any) {
	if m == nil {
		return
	}

	if m.IsSeparator {
		// If the item is a separator, nothing else matters
		builder.AddSeparator()
		return
	}

	var item any
	if m.Label != "" {
		// A menu item must have a non-empty label
		item = builder.AddMenuItem(m.Label, m.Tooltip, m.Disabled, m.NonProdOnly, m.Action, parent)
	}

	for _, child := range m.Items {
		// Recursively parse sub menu items
		parseMenuItem(&child, builder, item)
	}
}

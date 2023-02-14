package menu

func parseMenuData(m *MenuData, builder menuBuilder) {
	if m == nil {
		return
	}

	// Set top-level menu properties
	builder.setIcon(m.Icon)
	builder.setTooltip(m.Tooltip)

	for _, child := range m.Items {
		parseMenuItem(&child, builder, nil)
	}
}

func parseMenuItem(m *menuItemData, builder menuBuilder, parent any) {
	if m == nil {
		return
	}

	if m.IsSeparator {
		// If the item is a separator, nothing else matters
		builder.addSeparator()
		return
	}

	var item any
	if m.Label != "" {
		item = builder.addMenuItem(m.Label, m.Tooltip, m.Disabled, m.NonProdOnly, m.Action.Performer, parent)
	}

	if item == nil {
		// Menu item wasn't created, so we can't add child menu items
		return
	}

	for _, child := range m.Items {
		// Recursively parse sub menu items
		parseMenuItem(&child, builder, item)
	}
}

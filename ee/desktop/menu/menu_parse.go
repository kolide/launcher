package menu

func parseMenuData(m *MenuData, builder menuBuilder, parser textParser) {
	if m == nil {
		return
	}

	// Set top-level menu properties
	builder.setIcon(m.Icon)
	builder.setTooltip(m.Tooltip)

	for _, child := range m.Items {
		parseMenuItem(&child, builder, parser, nil)
	}
}

func parseMenuItem(m *menuItemData, builder menuBuilder, parser textParser, parent any) {
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
		// A menu item must have a non-empty label
		label, err := parser.parse(m.Label)
		if err != nil {
			return
		}

		item = builder.addMenuItem(label, m.Tooltip, m.Disabled, m.NonProdOnly, m.Action.Performer, parent)
	}

	if item == nil {
		// Menu item wasn't created, so we can't add child menu items
		return
	}

	for _, child := range m.Items {
		// Recursively parse sub menu items
		parseMenuItem(&child, builder, parser, item)
	}
}

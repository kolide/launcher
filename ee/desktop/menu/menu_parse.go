package menu

func parseMenuData(m *MenuData, builder MenuBuilder) {
	// Set top-level menu properties
	builder.SetIcon()
	builder.SetTooltip(m.Tooltip)

	for _, child := range m.Items {
		parseMenuItem(&child, builder, nil)
	}
}

func parseMenuItem(m *menuItemData, builder MenuBuilder, parent any) {
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

	if item == nil {
		// Menu item wasn't created, so we can't add child menu items
		return
	}

	for _, child := range m.Items {
		// Recursively parse sub menu items
		parseMenuItem(&child, builder, item)
	}
}

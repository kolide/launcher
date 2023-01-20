package menu

// Performs the RefreshMenu action
type actionRefreshMenu struct{}

func (a actionRefreshMenu) Perform(m *menu, parser textParser) {
	m.Build()
}

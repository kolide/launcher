package menu

// Performs the RefreshMenu action
type actionRefreshMenu struct{}

func (a actionRefreshMenu) Perform(m *menu) {
	m.Build()
}

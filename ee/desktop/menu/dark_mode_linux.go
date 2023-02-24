//go:build linux
// +build linux

package menu

func isDarkMode() bool {
	return false
}

// RegisterThemeChangeListener registers a listener to be notified when OS theme (dark/light) changes
func RegisterThemeChangeListener(f func()) {
	// no-op
}

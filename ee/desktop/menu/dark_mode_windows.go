//go:build windows
// +build windows

package menu

func isDarkMode() bool {
	return false
}

func RegisterThemeChangeListener(f func()) {
	// no-op
}

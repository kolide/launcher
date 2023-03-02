//go:build darwin
// +build darwin

package menu

func isDarkMode() bool {
	return systrayDarkMode
}

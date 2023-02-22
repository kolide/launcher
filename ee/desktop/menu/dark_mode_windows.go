//go:build windows
// +build windows

package menu

import (
	"golang.org/x/sys/windows/registry"
)

func isDarkMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil { // older version of Windows will not have this key
		return false
	}
	defer k.Close()

	useLight, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil { // older version of Windows will not have this value
		return false
	}

	return useLight == 0
}

func RegisterThemeChangeListener(f func()) {
	// no-op
}

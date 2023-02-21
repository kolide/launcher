//go:build darwin
// +build darwin

package menu

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation

#include <AppKit/AppKit.h>

bool isDarkMode();
void watchTheme();
*/
import (
	"C"
)
import (
	"sync"
)

var (
	themeChangeListeners []func()
	once                 sync.Once
)

func isDarkMode() bool {
	if C.isDarkMode() {
		return true
	}
	return false
}

// Registers a listener to be notified when OS theme (dark/light) changes
func RegisterThemeChangeListener(f func()) {
	themeChangeListeners = append(themeChangeListeners, f)

	// Ensure we register ourselves as an observer only once
	watchTheme := func() { C.watchTheme() }
	once.Do(watchTheme)
}

//export themeChanged
func themeChanged() {
	for _, listener := range themeChangeListeners {
		listener()
	}
}

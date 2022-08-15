//go:build windows
// +build windows

package systray

import (
	_ "embed"
)

//go:embed kolide-mark-only-purple-16x-32x.ico
var kolideSystrayIcon []byte

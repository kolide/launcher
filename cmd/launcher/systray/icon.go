//go:build darwin || linux
// +build darwin linux

package systray

import (
	_ "embed"
)

//go:embed kolide-mark-only-white.png
var kolideSystrayIcon []byte

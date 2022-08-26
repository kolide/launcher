//go:build darwin || linux
// +build darwin linux

package desktop

import (
	_ "embed"
)

//go:embed kolide-mark-only-white.png
var kolideDesktopIcon []byte

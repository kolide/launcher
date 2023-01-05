//go:build darwin || linux
// +build darwin linux

package assets

import (
	_ "embed"
)

var (
	//go:embed kolide.png
	KolideDesktopIcon []byte

	//go:embed kolide-debug.png
	KolideDebugDesktopIcon []byte

	KolideIconFilename string = "kolide.png"
)

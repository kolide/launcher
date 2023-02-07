//go:build windows
// +build windows

package assets

import (
	_ "embed"
)

var (
	//go:embed kolide.ico
	KolideDesktopIcon []byte

	//go:embed kolide-debug.ico
	KolideDebugDesktopIcon []byte

	KolideIconFilename string = "kolide.ico"
)

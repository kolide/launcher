//go:build windows
// +build windows

package desktop

import (
	_ "embed"
)

var (
	//go:embed kolide-windows-prod-icon.ico
	kolideDesktopIcon []byte

	//go:embed kolide-windows-non-prod-icon.ico
	kolideDesktopIconNonProd []byte
)

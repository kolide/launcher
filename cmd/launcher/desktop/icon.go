//go:build darwin || linux
// +build darwin linux

package desktop

import (
	_ "embed"
)

var (
	//go:embed kolide-prod-icon.png
	kolideDesktopIcon []byte

	//go:embed kolide-non-prod-icon.png
	kolideDesktopIconNonProd []byte
)

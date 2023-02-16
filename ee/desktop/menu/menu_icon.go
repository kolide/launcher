package menu

import (
	"runtime"

	"github.com/kolide/launcher/ee/ui/assets"
)

// getIcon returns the appropriate embedded asset for the requested menu icon type
func getIcon(icon menuIcon) []byte {
	switch icon {
	case TranslucentIcon:
		return chooseIcon(
			assets.MenubarTranslucentDarkmodeIco,
			assets.MenubarTranslucentDarkmodePng,
			assets.MenubarTranslucentLightmodeIco,
			assets.MenubarTranslucentLightmodePng,
			assets.MenubarTranslucentMonochromeIco,
			assets.MenubarTranslucentMonochromePng,
		)
	case DefaultIcon:
		return chooseIcon(
			assets.MenubarDefaultDarkmodeIco,
			assets.MenubarDefaultDarkmodePng,
			assets.MenubarDefaultLightmodeIco,
			assets.MenubarDefaultLightmodePng,
			assets.MenubarDefaultMonochromeIco,
			assets.MenubarDefaultMonochromePng,
		)
	case TriangleExclamationIcon:
		return chooseIcon(
			assets.MenubarTriangleExclamationDarkmodeIco,
			assets.MenubarTriangleExclamationDarkmodePng,
			assets.MenubarTriangleExclamationLightmodeIco,
			assets.MenubarTriangleExclamationLightmodePng,
			assets.MenubarTriangleExclamationMonochromeIco,
			assets.MenubarTriangleExclamationMonochromePng,
		)
	case CircleXIcon:
		return chooseIcon(
			assets.MenubarCircleXDarkmodeIco,
			assets.MenubarCircleXDarkmodePng,
			assets.MenubarCircleXLightmodeIco,
			assets.MenubarCircleXLightmodePng,
			assets.MenubarCircleXMonochromeIco,
			assets.MenubarCircleXMonochromePng,
		)
	default:
		return nil
	}
}

// chooseIcon chooses the correct icon data, based on the current OS theme
func chooseIcon(darkIco, darkPng, lightIco, lightPng, monochromeIco, monochromePng []byte) []byte {
	// TODO: Ignoring dark/light modes at the moment
	// Here is where we would attempt to detect light/dark mode and choose the right icon
	// See https://github.com/kolide/launcher/issues/1028
	if runtime.GOOS == "windows" {
		return monochromeIco
	}
	return monochromePng
}

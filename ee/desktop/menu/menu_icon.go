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
			assets.MenubarTranslucentLightmodeShadowIco,
			assets.MenubarTranslucentLightmodeShadowPng,
			assets.MenubarTranslucentMonochromeIco,
			assets.MenubarTranslucentMonochromePng,
		)
	case DefaultIcon:
		return chooseIcon(
			assets.MenubarDefaultDarkmodeIco,
			assets.MenubarDefaultDarkmodePng,
			assets.MenubarDefaultLightmodeIco,
			assets.MenubarDefaultLightmodePng,
			assets.MenubarDefaultLightmodeShadowIco,
			assets.MenubarDefaultLightmodeShadowPng,
			assets.MenubarDefaultMonochromeIco,
			assets.MenubarDefaultMonochromePng,
		)
	case TriangleExclamationIcon:
		return chooseIcon(
			assets.MenubarTriangleExclamationDarkmodeIco,
			assets.MenubarTriangleExclamationDarkmodePng,
			assets.MenubarTriangleExclamationLightmodeIco,
			assets.MenubarTriangleExclamationLightmodePng,
			assets.MenubarTriangleExclamationLightmodeShadowIco,
			assets.MenubarTriangleExclamationLightmodeShadowPng,
			assets.MenubarTriangleExclamationMonochromeIco,
			assets.MenubarTriangleExclamationMonochromePng,
		)
	case CircleXIcon:
		return chooseIcon(
			assets.MenubarCircleXDarkmodeIco,
			assets.MenubarCircleXDarkmodePng,
			assets.MenubarCircleXLightmodeIco,
			assets.MenubarCircleXLightmodePng,
			assets.MenubarCircleXLightmodeShadowIco,
			assets.MenubarCircleXLightmodeShadowPng,
			assets.MenubarCircleXMonochromeIco,
			assets.MenubarCircleXMonochromePng,
		)
	default:
		return nil
	}
}

// chooseIcon chooses the appropriate icon data for the OS
func chooseIcon(darkIco, darkPng, lightIco, lightPng, shadowIco, shadowPng, monochromeIco, monochromePng []byte) []byte {
	if runtime.GOOS == "windows" {
		return shadowIco
	}
	return darkOrLight(darkPng, lightPng)
}

// darkOrLight returns the dark icon data if the OS theme is dark, otherwise defaults to light
func darkOrLight(dark, light []byte) []byte {
	if isDarkMode() {
		return dark
	}
	return light
}

package menu

import (
	"runtime"

	"github.com/kolide/launcher/v2/ee/ui/assets"
)

// getIcon returns the appropriate embedded asset for the requested menu icon type,
// using the Device Trust icons when deviceTrustRebrand is enabled and the Kolide icons otherwise
func getIcon(icon menuIcon, deviceTrustRebrand bool) []byte {
	if deviceTrustRebrand {
		return getDeviceTrustIcon(icon)
	}
	return getKolideIcon(icon)
}

// getKolideIcon returns the legacy Kolide-branded asset for the requested menu icon type
func getKolideIcon(icon menuIcon) []byte {
	switch icon {
	case TranslucentIcon:
		return chooseIcon(
			assets.MenubarTranslucentDarkmodePng,
			assets.MenubarTranslucentLightmodePng,
			assets.MenubarTranslucentLightmodeShadowIco,
			assets.MenubarTranslucentLightmodeShadowPng,
		)
	case TriangleExclamationIcon:
		return chooseIcon(
			assets.MenubarTriangleExclamationDarkmodePng,
			assets.MenubarTriangleExclamationLightmodePng,
			assets.MenubarTriangleExclamationLightmodeShadowIco,
			assets.MenubarTriangleExclamationLightmodeShadowPng,
		)
	case CircleXIcon:
		return chooseIcon(
			assets.MenubarCircleXDarkmodePng,
			assets.MenubarCircleXLightmodePng,
			assets.MenubarCircleXLightmodeShadowIco,
			assets.MenubarCircleXLightmodeShadowPng,
		)
	case CircleDotIcon:
		return chooseIcon(
			assets.MenubarCircleDotDarkmodePng,
			assets.MenubarCircleDotLightmodePng,
			assets.MenubarCircleDotLightmodeShadowIco,
			assets.MenubarCircleDotLightmodeShadowPng,
		)
	case DefaultIcon:
		fallthrough
	default:
		return chooseIcon(
			assets.MenubarDefaultDarkmodePng,
			assets.MenubarDefaultLightmodePng,
			assets.MenubarDefaultLightmodeShadowIco,
			assets.MenubarDefaultLightmodeShadowPng,
		)
	}
}

// getDeviceTrustIcon returns the Device Trust-branded asset for the requested menu icon type
func getDeviceTrustIcon(icon menuIcon) []byte {
	switch icon {
	case TranslucentIcon:
		return chooseIcon(
			assets.DeviceTrustMenubarTranslucentDarkmodePng,
			assets.DeviceTrustMenubarTranslucentLightmodePng,
			assets.DeviceTrustMenubarTranslucentLightmodeShadowIco,
			assets.DeviceTrustMenubarTranslucentLightmodeShadowPng,
		)
	case TriangleExclamationIcon:
		return chooseIcon(
			assets.DeviceTrustMenubarTriangleExclamationDarkmodePng,
			assets.DeviceTrustMenubarTriangleExclamationLightmodePng,
			assets.DeviceTrustMenubarTriangleExclamationLightmodeShadowIco,
			assets.DeviceTrustMenubarTriangleExclamationLightmodeShadowPng,
		)
	case CircleXIcon:
		return chooseIcon(
			assets.DeviceTrustMenubarCircleXDarkmodePng,
			assets.DeviceTrustMenubarCircleXLightmodePng,
			assets.DeviceTrustMenubarCircleXLightmodeShadowIco,
			assets.DeviceTrustMenubarCircleXLightmodeShadowPng,
		)
	case CircleDotIcon:
		return chooseIcon(
			assets.DeviceTrustMenubarCircleDotDarkmodePng,
			assets.DeviceTrustMenubarCircleDotLightmodePng,
			assets.DeviceTrustMenubarCircleDotLightmodeShadowIco,
			assets.DeviceTrustMenubarCircleDotLightmodeShadowPng,
		)
	case DefaultIcon:
		fallthrough
	default:
		return chooseIcon(
			assets.DeviceTrustMenubarDefaultDarkmodePng,
			assets.DeviceTrustMenubarDefaultLightmodePng,
			assets.DeviceTrustMenubarDefaultLightmodeShadowIco,
			assets.DeviceTrustMenubarDefaultLightmodeShadowPng,
		)
	}
}

// chooseIcon chooses the appropriate icon data for the OS
func chooseIcon(darkPng, lightPng, shadowIco, shadowPng []byte) []byte {
	// Windows and Linux don't observe dark/light modes and use the colored icons with shadows
	if runtime.GOOS == "windows" {
		return shadowIco
	}
	if runtime.GOOS == "linux" {
		return shadowPng
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

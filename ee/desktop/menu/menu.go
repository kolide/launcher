package menu

import (
	"fmt"
	"os"

	"fyne.io/systray"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/desktop"
	"github.com/kolide/launcher/ee/desktop/assets"
)

func Init(hostname string) {
	onReady := func() {
		systray.SetTemplateIcon(assets.KolideDesktopIcon, assets.KolideDesktopIcon)
		systray.SetTooltip("Kolide")

		versionItem := systray.AddMenuItem(fmt.Sprintf("Version %s", version.Version().Version), "")
		versionItem.Disable()

		// if prod environment, return
		if hostname == "k2device-preprod.kolide.com" || hostname == "k2device.kolide.com" {
			return
		}

		// in non prod environment
		systray.SetTemplateIcon(assets.KolideDebugDesktopIcon, assets.KolideDebugDesktopIcon)
		systray.AddSeparator()
		systray.AddMenuItem("--- DEBUG ---", "").Disable()
		systray.AddMenuItem(fmt.Sprintf("Hostname: %s", hostname), "").Disable()
		systray.AddMenuItem(fmt.Sprintf("Socket Path: %s", desktop.DesktopSocketPath(os.Getpid())), "").Disable()
	}

	systray.Run(onReady, func() {})
}

func Shutdown() {
	systray.Quit()
}

package menu

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/kolide/kit/version"

	"github.com/kolide/launcher/ee/desktop/assets"
)

func Init(hostname string) {
	a := app.New()

	if desk, ok := a.(desktop.App); ok {
		var items []*fyne.MenuItem

		// Creates the menu populated with the menu items, and starts the event loop
		setMenuAndRun := func() {
			menu := fyne.NewMenu("Kolide", items...)
			desk.SetSystemTrayMenu(menu)

			a.Run()
		}
		defer setMenuAndRun()

		desk.SetSystemTrayIcon(fyne.NewStaticResource("KolideDesktopIcon", assets.KolideDesktopIcon))

		versionItem := fyne.NewMenuItem(fmt.Sprintf("Version %s", version.Version().Version), func() {})
		versionItem.Disabled = true
		items = append(items, versionItem)

		// if prod environment, return
		if hostname == "k2device-preprod.kolide.com" || hostname == "k2device.kolide.com" {
			return
		}

		// in non prod environment
		desk.SetSystemTrayIcon(fyne.NewStaticResource("KolideDebugDesktopIcon", assets.KolideDebugDesktopIcon))

		// debug menu items section
		items = append(items, fyne.NewMenuItemSeparator())
		items = append(items, fyne.NewMenuItem("--- DEBUG ---", func() {}))
		items = append(items, fyne.NewMenuItem(fmt.Sprintf("Hostname: %s", hostname), func() {}))
	}
}

func Shutdown() {
	fyne.CurrentApp().Quit()
}

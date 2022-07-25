package systray

import (
	_ "embed"

	"github.com/getlantern/systray"
)

//go:embed systray-kolide.ico
var kolideSystrayIcon []byte

func RunSystray(args []string) error {

	onReady := func() {
		systray.SetIcon(kolideSystrayIcon)
		systray.SetTooltip("Kolide")
	}

	systray.Run(onReady, func() {})

	return nil
}

package systray

import (
	"fmt"
	"os"
	"time"

	"fyne.io/systray"
	"github.com/kolide/kit/version"
)

func RunSystray(args []string) error {

	go exitWhenParentGone()

	onReady := func() {
		systray.SetTemplateIcon(kolideSystrayIcon, kolideSystrayIcon)
		systray.SetTooltip("Kolide")

		versionItem := systray.AddMenuItem(fmt.Sprintf("Version %s", version.Version().Version), "")
		versionItem.Disable()
	}

	systray.Run(onReady, func() {})
	return nil
}

// continuously monitor for ppid and exit if parent process terminates
func exitWhenParentGone() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	f := func() {
		if os.Getppid() <= 1 {
			fmt.Println("parent process is gone, exiting")
			systray.Quit()
			os.Exit(1)
		}
	}

	f()

	for {
		select {
		case <-ticker.C:
			f()
		}
	}
}

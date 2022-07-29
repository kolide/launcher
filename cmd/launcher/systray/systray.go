package systray

import (
	_ "embed"
	"fmt"
	"os"
	"time"

	"fyne.io/systray"
	"github.com/kolide/kit/version"
)

//go:embed kolide-mark-only-black-32x.ico
var kolideSystrayIcon []byte

func RunSystray(args []string) error {

	go exitWhenParentGone()

	onReady := func() {
		systray.SetIcon(kolideSystrayIcon)
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

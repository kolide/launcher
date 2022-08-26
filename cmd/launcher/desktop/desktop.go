package desktop

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/kolide/kit/version"
)

func RunDesktop(args []string) error {

	go exitWhenParentGone()
	go handleSignals()

	onReady := func() {
		systray.SetTemplateIcon(kolideDesktopIcon, kolideDesktopIcon)
		systray.SetTooltip("Kolide")

		versionItem := systray.AddMenuItem(fmt.Sprintf("Version %s", version.Version().Version), "")
		versionItem.Disable()
	}

	systray.Run(onReady, func() {})
	return nil
}

func handleSignals() {
	signalsToHandle := []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	signals := make(chan os.Signal, len(signalsToHandle))
	signal.Notify(signals, signalsToHandle...)
	sig := <-signals
	fmt.Println(fmt.Sprintf("\nreceived %s signal, exiting", sig))
	systray.Quit()
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

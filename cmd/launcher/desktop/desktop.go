package desktop

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/kolide/kit/version"
	"github.com/shirou/gopsutil/process"
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
	fmt.Printf("\nreceived %s signal, exiting", sig)
	systray.Quit()
}

// continuously monitor for ppid and exit if parent process terminates
func exitWhenParentGone() {
	for ; true; <-time.NewTicker(2 * time.Second).C {
		ppid := os.Getppid()

		if ppid <= 1 {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		exists, err := process.PidExistsWithContext(ctx, int32(ppid))

		// pretty sure this is not needed since it should call cancel on it's won when time is exceeded
		// https://cs.opensource.google/go/go/+/master:src/context/context.go;l=456?q=func%20WithDeadline&ss=go%2Fgo
		// but the linter and the context.WithTimeout docs say to do it
		cancel()
		if err != nil || !exists {
			break
		}
	}

	fmt.Print("\nparent process is gone, exiting")
	systray.Quit()
	os.Exit(1)
}

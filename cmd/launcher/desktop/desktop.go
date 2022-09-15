package desktop

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/desktop"
	"github.com/kolide/launcher/ee/desktop/assets"
	"github.com/kolide/launcher/ee/desktop/server"
	"github.com/shirou/gopsutil/process"
)

func RunDesktop(args []string) error {

	go exitWhenParentGone()
	go handleSignals()

	go func() {
		if err := server.Start(); err != nil {
			//TODO: log this
		}
	}()

	flagset := flag.NewFlagSet("launcher desktop", flag.ExitOnError)
	var (
		flhostname = flagset.String(
			"hostname",
			"",
			"hostname launcher is connected to",
		)
	)
	if err := flagset.Parse(args); err != nil {
		return err
	}

	onReady := func() {
		systray.SetTemplateIcon(assets.KolideDesktopIcon, assets.KolideDesktopIcon)
		systray.SetTooltip("Kolide")

		versionItem := systray.AddMenuItem(fmt.Sprintf("Version %s", version.Version().Version), "")
		versionItem.Disable()

		// if prod environment, return
		if *flhostname == "k2device-preprod.kolide.com" || *flhostname == "k2device.kolide.com" {
			return
		}

		// in non prod environment
		systray.SetTemplateIcon(assets.KolideDebugDesktopIcon, assets.KolideDebugDesktopIcon)
		systray.AddSeparator()
		systray.AddMenuItem("--- DEBUG ---", "").Disable()
		systray.AddMenuItem(fmt.Sprintf("Hostname: %s", *flhostname), "").Disable()
		systray.AddMenuItem(fmt.Sprintf("Socket Path: %s", desktop.DesktopSocketPath(os.Getpid())), "").Disable()
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
	ticker := time.NewTicker(2 * time.Second)

	for ; true; <-ticker.C {
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

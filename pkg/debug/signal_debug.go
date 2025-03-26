//go:build !windows
// +build !windows

package debug

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kolide/launcher/ee/gowrapper"
)

const debugSignal = syscall.SIGUSR1

// AttachDebugHandler attaches a signal handler that toggles the debug server
// state when SIGUSR1 is sent to the process.
func AttachDebugHandler(addrPath string, slogger *slog.Logger) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, debugSignal)
	gowrapper.Go(context.TODO(), slogger, func() {
		for {
			// Start server on first signal
			<-sig
			serv, err := startDebugServer(addrPath, slogger)
			if err != nil {
				slogger.Log(context.TODO(), slog.LevelInfo,
					"starting debug server",
					"err", err,
				)
				continue
			}

			// Stop server on next signal
			<-sig
			if err := serv.Shutdown(context.Background()); err != nil {
				slogger.Log(context.TODO(), slog.LevelInfo,
					"error shutting down debug server",
					"err", err,
				)
				continue
			}

			slogger.Log(context.TODO(), slog.LevelInfo,
				"shutdown debug server",
			)
		}
	})

}

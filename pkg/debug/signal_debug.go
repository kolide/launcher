// +build !windows

package debug

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

const debugSignal = syscall.SIGUSR1

// AttachDebugHandler attaches a signal handler that toggles the debug server
// state when SIGUSR1 is sent to the process.
func AttachDebugHandler(addrPath string, logger log.Logger) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, debugSignal)
	go func() {
		for {
			// Start server on first signal
			<-sig
			serv, err := startDebugServer(addrPath, logger)
			if err != nil {
				level.Info(logger).Log(
					"msg", "starting debug server",
					"err", err,
				)
				continue
			}

			// Stop server on next signal
			<-sig
			if err := serv.Shutdown(context.Background()); err != nil {
				level.Info(logger).Log(
					"msg", "error shutting down debug server",
					"err", err,
				)
				continue
			}

			level.Info(logger).Log(
				"msg", "shutdown debug server",
			)
		}
	}()
}

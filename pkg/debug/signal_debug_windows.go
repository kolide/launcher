//go:build windows
// +build windows

package debug

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// AttachDebugHandler looks for a sentinal file and uses that to
// trigger a debug server. Windows does not support SIGUSR1, so this
// alternate mechanism is used.
func AttachDebugHandler(triggerPath string, addrPath string, logger log.Logger) {
	go func() {
		_, err := startDebugServer(addrPath, logger)
		if err != nil {
			level.Info(logger).Log(
				"msg", "error starting debug server",
				"err", err,
			)
		}
		level.Info(logger).Log("msg", "starting debug server")
	}()

	if false {
		go func() {
			var serv *http.Server
			var err error

			for {
				_, statErr := os.Stat(triggerPath)
				switch {
				case statErr == nil:
					if serv != nil {
						continue
					}

					serv, err = startDebugServer(addrPath, logger)
					if err != nil {
						level.Info(logger).Log(
							"msg", "error starting debug server",
							"err", err,
						)
						continue
					}
					level.Info(logger).Log("msg", "starting debug server")

				case os.IsNotExist(statErr):
					if serv == nil {
						continue
					}

					if err := serv.Shutdown(context.Background()); err != nil {
						level.Info(logger).Log(
							"msg", "error shutting down debug server",
							"err", err,
						)
						continue
					}

					level.Info(logger).Log("msg", "stopped debug server")
				default:
					level.Info(logger).Log(
						"msg", "Error checking debug server trigger",
						"err", statErr,
					)
				}

				time.Sleep(30 * time.Second)
			}

		}()
	}
}

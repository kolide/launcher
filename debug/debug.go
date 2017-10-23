package debug

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/e-dard/netbug"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const debugSignal = syscall.SIGUSR1

// AttachDebugHandler will attach a signal handler that will toggle the debug
// server state when SIGUSR1 is sent to the process.
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
			} else {
				level.Info(logger).Log(
					"msg", "shutdown debug server",
				)
			}
		}
	}()
}

func startDebugServer(addrPath string, logger log.Logger) (*http.Server, error) {
	// Generate new (random) token to use for debug server auth
	token, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Wrap(err, "generating debug token")
	}

	// Start the debug server
	r := http.NewServeMux()
	netbug.RegisterAuthHandler(token.String(), "/debug/", r)
	serv := http.Server{
		Handler: r,
	}
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, errors.Wrap(err, "opening socket")
	}

	go func() {
		if err := serv.Serve(listener); err != nil && err != http.ErrServerClosed {
			level.Info(logger).Log("msg", "debug server failed", "err", err)
		}
	}()

	addr := fmt.Sprintf("http://%s/debug/?token=%s", listener.Addr().String(), token.String())
	// Write the address to a file for easy access by users
	if err := ioutil.WriteFile(addrPath, []byte(addr), 0600); err != nil {
		return nil, errors.Wrap(err, "writing debug address")
	}

	level.Info(logger).Log(
		"msg", "debug server started",
		"addr", addr,
	)

	return &serv, nil
}

package main

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	runnerserver "github.com/kolide/launcher/ee/desktop/runner/server"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func Test_desktopMonitorParentProcess(t *testing.T) { //nolint:paralleltest
	runnerServer, err := runnerserver.New(multislogger.NewNopLogger(), nil, nil)
	require.NoError(t, err)

	// start server
	go func() {
		if err := runnerServer.Serve(); err != nil {
			require.ErrorIs(t, err, http.ErrServerClosed)
		}
	}()

	// register client and get token
	token := runnerServer.RegisterClient("0")

	monitorInterval := 2 * time.Second
	var logBytes threadsafebuffer.ThreadSafeBuffer

	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	// Start up monitoring the parent process, waiting just a moment for the server to start
	time.Sleep(monitorInterval * 2)
	monitorShutdownChan := make(chan struct{}, 1)
	go func() {
		monitorParentProcess(slogger, runnerServer.Url(), token, monitorInterval)
		monitorShutdownChan <- struct{}{}
	}()

	// let it run for a few intervals and make sure there is no error
	time.Sleep(monitorInterval * 4)

	// we should succeed now, nothing should be in the log
	require.Empty(t, logBytes.String())

	// stop the server -- we should now start getting errors
	require.NoError(t, runnerServer.Shutdown(t.Context()))
	time.Sleep(monitorInterval * 8)

	// `monitorParentProcess` should perform some retries during this time
	require.Contains(t, logBytes.String(), "will retry")

	// `monitorParentProcess` should exit
	select {
	case <-monitorShutdownChan:
		// monitorParentProcess returned
	case <-time.After(8 * monitorInterval):
		t.Errorf("monitorParentProcess did not exit after parent server shutdown: logs:\n%s\n", logBytes.String())
		t.FailNow()
	}
}

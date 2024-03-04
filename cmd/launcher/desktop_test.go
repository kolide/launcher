package main

import (
	"context"
	"io"
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

	// register client and get token
	token := runnerServer.RegisterClient("0")

	monitorInterval := 250 * time.Millisecond
	var logBytes threadsafebuffer.ThreadSafeBuffer

	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	go func() {
		monitorParentProcess(slogger, runnerServer.Url(), token, monitorInterval)
	}()

	time.Sleep(monitorInterval * 2)

	// should retry
	require.Contains(t, logBytes.String(), "will retry")

	// start server
	go func() {
		if err := runnerServer.Serve(); err != nil {
			require.ErrorIs(t, err, http.ErrServerClosed)
		}
	}()

	// wait a moment for server to start
	time.Sleep(monitorInterval * 2)

	// clear the log
	io.Copy(io.Discard, &logBytes)
	// let it run for a few intervals and make sure there is no error
	time.Sleep(monitorInterval * 4)

	// we should succeed now, nothing should be in the log
	require.Empty(t, logBytes.String())

	// stop the server, should now start getting errors
	require.NoError(t, runnerServer.Shutdown(context.Background()))
	time.Sleep(monitorInterval * 8)

	// should retry
	require.Contains(t, logBytes.String(), "will retry")

	// should exit
	require.Contains(t, logBytes.String(), "exiting")
}

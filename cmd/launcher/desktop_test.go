package main

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/desktop/runner/server"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func Test_desktopMonitorParentProcess(t *testing.T) { //nolint:paralleltest
	rootServer, err := server.New(log.NewNopLogger(), nil)
	require.NoError(t, err)

	// register client and get token
	token := rootServer.RegisterClient("0")

	monitorInterval := 250 * time.Millisecond
	var logBytes threadsafebuffer.ThreadSafeBuffer

	go func() {
		monitorParentProcess(log.NewLogfmtLogger(&logBytes), rootServer.Url(), token, monitorInterval)
	}()

	time.Sleep(monitorInterval * 2)

	// should retry
	require.Contains(t, logBytes.String(), "will retry")

	// start server
	go func() {
		if err := rootServer.Serve(); err != nil {
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
	require.NoError(t, rootServer.Shutdown(context.Background()))
	time.Sleep(monitorInterval * 8)

	// should retry
	require.Contains(t, logBytes.String(), "will retry")

	// should exit
	require.Contains(t, logBytes.String(), "exiting")
}

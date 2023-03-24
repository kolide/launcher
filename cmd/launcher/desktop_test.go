package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func Test_desktopMonitorParentProcess(t *testing.T) { //nolint:paralleltest
	mux := http.NewServeMux()

	server := http.Server{
		Handler: mux,
	}

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	go func() {
		server.Serve(listener)
	}()

	monitorInterval := 250 * time.Millisecond
	monitorEndpoint := ulid.New()
	monitorURL := fmt.Sprintf("http://%s/%s", listener.Addr().String(), monitorEndpoint)
	var logBytes threadsafebuffer.ThreadSafeBuffer

	go func() {
		monitorParentProcess(log.NewLogfmtLogger(&logBytes), monitorURL, monitorInterval)
	}()

	time.Sleep(monitorInterval)

	// request should fail since there is not handler for the endpoint
	// make sure we retry
	require.Contains(t, logBytes.String(), "will retry")

	// add a handler to the endpoint
	mux.HandleFunc(fmt.Sprintf("/%s", monitorEndpoint), func(w http.ResponseWriter, r *http.Request) {})

	// clear log
	io.Copy(io.Discard, &logBytes)
	time.Sleep(monitorInterval * 8)

	// we should succeed now, nothing should be in the log
	require.Empty(t, logBytes.String())

	// stop the server, should now start getting errors
	require.NoError(t, server.Shutdown(context.Background()))
	time.Sleep(monitorInterval * 8)

	// should retry
	require.Contains(t, logBytes.String(), "will retry")

	// should exit
	require.Contains(t, logBytes.String(), "exiting")
}

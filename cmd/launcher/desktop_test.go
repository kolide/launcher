package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func Test_desktopMonitorParentProcess(t *testing.T) {
	t.Parallel()

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

	// make sure we retry
	time.Sleep(monitorInterval)
	require.Contains(t, logBytes.String(), "will retry")

	// add a handler to the endpoint
	mux.HandleFunc(fmt.Sprintf("/%s", monitorEndpoint), func(w http.ResponseWriter, r *http.Request) {})
	time.Sleep(monitorInterval * 8)

	// make sure we don't exit
	require.NotContains(t, logBytes.String(), "exiting")

	// stop the server
	require.NoError(t, server.Shutdown(context.Background()))
	time.Sleep(monitorInterval * 8)

	// make sure we exit
	require.Contains(t, logBytes.String(), "exiting")
}

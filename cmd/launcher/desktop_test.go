//go:build !race
// +build !race

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func Test_desktopMonitorParentProcess(t *testing.T) { //nolint:paralleltest
	// calling httptest.NewServer  by itself fails when parallel on windows with
	// panic: httptest: failed to listen on a port: listen tcp6 [::1]:0: socket: The requested service provider could not be loaded or initialized.

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			require.NoError(t, r.Body.Close())
		}
	}))

	monitorInterval := 250 * time.Millisecond
	var logBytes threadsafebuffer.ThreadSafeBuffer

	go func() {
		monitorParentProcess(log.NewLogfmtLogger(&logBytes), server.URL, monitorInterval)
	}()

	time.Sleep(8 * monitorInterval)
	//require.Empty(t, logBytes.String())
	require.Contains(t, logBytes.String(), "could not connect to parent, will back off and retry")

	server.Start()

	server.Close()
	time.Sleep(8 * monitorInterval)
	require.Contains(t, logBytes.String(), "could not connect to parent")
}

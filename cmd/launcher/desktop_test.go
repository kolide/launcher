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

func Test_desktopMonitorParentProcess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			require.NoError(t, r.Body.Close())
		}
	}))

	monitorInterval := 1 * time.Second
	var logBytes threadsafebuffer.ThreadSafeBuffer

	go func() {
		monitorParentProcess(log.NewLogfmtLogger(&logBytes), server.URL, monitorInterval)
	}()

	time.Sleep(3 * monitorInterval)
	require.Empty(t, logBytes.String())

	server.Close()

	time.Sleep(3 * monitorInterval)
	require.Contains(t, logBytes.String(), "could not connect to parent, exiting")
}

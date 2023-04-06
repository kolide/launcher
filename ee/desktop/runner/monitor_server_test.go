package runner

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitorServer(t *testing.T) {
	t.Parallel()

	monitorServer, err := newMonitorServer()
	require.NoError(t, err)

	go func() {
		if err := monitorServer.serve(); err != nil {
			require.ErrorIs(t, err, http.ErrServerClosed)
		}
	}()

	// add some endpoints, make sure they work
	for i := 0; i < 5; i++ {
		endpoint := monitorServer.newEndpoint()
		response, err := http.Get(endpoint)
		if response.Body != nil {
			require.NoError(t, response.Body.Close())
		}
		require.NoError(t, err)
		require.Equal(t, response.StatusCode, http.StatusOK)
	}

	// make a bad request, make sure we get an error
	response, err := http.Get(fmt.Sprintf("https://%s/%s", monitorServer.listener.Addr().String(), "not_real"))
	if response != nil && response.Body != nil {
		require.NoError(t, response.Body.Close())
	}
	require.Error(t, err)

	require.NoError(t, monitorServer.Shutdown(context.Background()))
}

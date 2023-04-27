package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/authedclient"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRootServer(t *testing.T) {
	t.Parallel()

	mockSack := mocks.NewKnapsack(t)
	mockSack.On("SetControlRequestIntervalOverride", mock.Anything, mock.Anything)

	monitorServer, err := New(log.NewNopLogger(), mockSack)
	require.NoError(t, err)

	go func() {
		if err := monitorServer.Serve(); err != nil {
			require.ErrorIs(t, err, http.ErrServerClosed)
		}
	}()

	// register proc and get token
	token := monitorServer.RegisterClient("0")
	client := authedclient.New(token, 1*time.Second)

	response, err := client.Get(endpointUrl(monitorServer.Url(), HealthCheckEndpoint))
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	response, err = client.Get(endpointUrl(monitorServer.Url(), MenuOpenedEndpoint))
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	// deregister and make sure we get unauthorized status codes
	monitorServer.DeRegisterClient("0")

	response, err = client.Get(endpointUrl(monitorServer.Url(), HealthCheckEndpoint))
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)

	response, err = client.Get(endpointUrl(monitorServer.Url(), MenuOpenedEndpoint))
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)

	require.NoError(t, monitorServer.Shutdown(context.Background()))
}

func endpointUrl(url, endpoint string) string {
	return fmt.Sprintf("%s%s", url, endpoint)
}

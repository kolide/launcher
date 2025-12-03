package server

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/types/mocks"
	servermocks "github.com/kolide/launcher/ee/desktop/runner/server/mocks"
	"github.com/kolide/launcher/pkg/authedclient"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRootServer(t *testing.T) {
	t.Parallel()

	mockSack := mocks.NewKnapsack(t)
	mockSack.On("SetControlRequestIntervalOverride", mock.Anything, mock.Anything)
	mockSack.On("SetDistributedForwardingIntervalOverride", mock.Anything, mock.Anything)

	messenger := servermocks.NewMessenger(t)

	monitorServer, err := New(multislogger.NewNopLogger(), mockSack, messenger)
	require.NoError(t, err)

	go func() {
		if err := monitorServer.Serve(); err != nil {
			require.ErrorIs(t, err, http.ErrServerClosed)
		}
	}()

	// register proc and get token
	token := monitorServer.RegisterClient("0")
	client := authedclient.New(token, 1*time.Second)

	response, err := client.Get(endpointUrl(monitorServer.Url(), HealthCheckEndpoint)) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	response, err = client.Get(endpointUrl(monitorServer.Url(), MenuOpenedEndpoint)) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	response, err = client.Post(endpointUrl(monitorServer.Url(), MessageEndpoint), "application/json", nil) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)

	response, err = client.Post(endpointUrl(monitorServer.Url(), MessageEndpoint), "application/json", bytes.NewReader([]byte(`{"method":""}`))) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)

	messenger.On("SendMessage", "test", mock.Anything).Return(errors.New("some error")).Once()
	response, err = client.Post(endpointUrl(monitorServer.Url(), MessageEndpoint), "application/json", bytes.NewReader([]byte(`{"method":"test"}`))) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, response.StatusCode)

	messenger.On("SendMessage", "test", mock.Anything).Return(nil).Once()
	response, err = client.Post(endpointUrl(monitorServer.Url(), MessageEndpoint), "application/json", bytes.NewReader([]byte(`{"method":"test"}`))) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	// deregister and make sure we get unauthorized status codes
	monitorServer.DeRegisterClient("0")

	response, err = client.Get(endpointUrl(monitorServer.Url(), HealthCheckEndpoint)) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)

	response, err = client.Get(endpointUrl(monitorServer.Url(), MenuOpenedEndpoint)) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)

	response, err = client.Post(endpointUrl(monitorServer.Url(), MessageEndpoint), "application/json", nil) //nolint:noctx // We don't care about this in tests
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)

	require.NoError(t, monitorServer.Shutdown(t.Context()))
}

func endpointUrl(url, endpoint string) string {
	return fmt.Sprintf("%s%s", url, endpoint)
}

package control

import (
	"net/http"
	"testing"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	// Set up mock knapsack
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlServerURL).Return()
	mockKnapsack.On("ControlServerURL").Return("example.com").Once() // only once, so that we can test updating the URL

	// Set up control client
	testClient, err := NewControlHTTPClient(http.DefaultClient, mockKnapsack, multislogger.NewNopLogger())
	require.NoError(t, err, "initializing control client")

	// Trigger flag change
	updatedUrl := "example.test"
	mockKnapsack.On("ControlServerURL").Return(updatedUrl)
	testClient.FlagsChanged(t.Context(), keys.ControlServerURL)

	// Confirm url has updated
	testClient.baseURLLock.RLock()
	require.Equal(t, updatedUrl, testClient.baseURL.Host)
	require.Equal(t, "https", testClient.baseURL.Scheme)
	testClient.baseURLLock.RUnlock()

	mockKnapsack.AssertExpectations(t)
}

func TestFlagsChanged_WithDisableTLS(t *testing.T) {
	t.Parallel()

	// Set up mock knapsack
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlServerURL).Return()
	mockKnapsack.On("ControlServerURL").Return("example.com").Once() // only once, so that we can test updating the URL

	// Set up control client
	testClient, err := NewControlHTTPClient(http.DefaultClient, mockKnapsack, multislogger.NewNopLogger(), WithDisableTLS())
	require.NoError(t, err, "initializing control client")

	// Trigger flag change
	updatedUrl := "example.test"
	mockKnapsack.On("ControlServerURL").Return(updatedUrl)
	testClient.FlagsChanged(t.Context(), keys.ControlServerURL)

	// Confirm url has updated
	testClient.baseURLLock.RLock()
	require.Equal(t, updatedUrl, testClient.baseURL.Host)
	require.Equal(t, "http", testClient.baseURL.Scheme)
	testClient.baseURLLock.RUnlock()

	mockKnapsack.AssertExpectations(t)
}

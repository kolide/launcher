package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestForceNoChunkedEncoding(t *testing.T) {
	t.Parallel()

	req := &http.Request{
		Method: "POST",
		Body:   io.NopCloser(bytes.NewBufferString("Hello World")),
	}

	// Check no ContentLength
	require.Equal(t, int64(0), req.ContentLength)

	forceNoChunkedEncoding(multislogger.NewNopLogger())(t.Context(), req)

	// Check that we _now_ have ContentLength
	require.Equal(t, int64(11), req.ContentLength)

	// Check contents are still as expected
	content := &bytes.Buffer{}
	written, err := io.Copy(content, req.Body)
	require.NoError(t, err)
	require.Equal(t, int64(11), written)
	require.Equal(t, "Hello World", content.String())
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	// set up mock knapsack
	mockKnapsack := mocks.NewKnapsack(t)
	mockKnapsack.On("KolideServerURL").Return("example.test").Once() // starting value
	mockKnapsack.On("InsecureTransportTLS").Return(true)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.KolideServerURL).Return()

	testClient, err := NewJSONRPCClient(mockKnapsack)
	require.NoError(t, err)

	// set up JSON-RPC test server with updated url
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		respBody := healthcheckResponse{
			Status: 0,
		}
		respBodyRaw, err := json.Marshal(respBody)
		require.NoError(t, err)
		resp := jsonrpc.Response{
			Result: respBodyRaw,
		}
		respJson, err := json.Marshal(resp)
		require.NoError(t, err)
		w.Write(respJson)
	}))
	u, err := url.Parse(testServer.URL)
	require.NoError(t, err)
	mockKnapsack.On("KolideServerURL").Return(u.Host)

	// Call FlagsChanged, and confirm that the endpoint URL has updated
	testClient.FlagsChanged(t.Context(), keys.KolideServerURL)
	_, err = testClient.CheckHealth(t.Context())
	require.NoError(t, err)

	mockKnapsack.AssertExpectations(t)
}

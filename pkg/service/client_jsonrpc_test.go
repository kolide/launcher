package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/logger"
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

	forceNoChunkedEncoding(context.TODO(), req)

	// Check that we _now_ have ContentLength
	require.Equal(t, int64(11), req.ContentLength)

	// Check contents are still as expected
	content := &bytes.Buffer{}
	len, err := io.Copy(content, req.Body)
	require.NoError(t, err)
	require.Equal(t, int64(11), len)
	require.Equal(t, "Hello World", content.String())
}

func TestDeviceDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		f    func(client KolideService) error
	}{
		{
			name: "RequestEnrollment",
			f: func(client KolideService) error {
				_, _, err := client.RequestEnrollment(context.TODO(), "enroll_secret", "host_identifier", EnrollmentDetails{})
				return err
			},
		},
		{
			name: "RequestConfig",
			f: func(client KolideService) error {
				_, _, err := client.RequestConfig(context.TODO(), "node_key")
				return err
			},
		},
		{
			name: "PublishLogs",
			f: func(client KolideService) error {
				_, _, _, err := client.PublishLogs(context.Background(), "node_key", logger.LogTypeStatus, nil)
				return err
			},
		},
		{
			name: "RequestQueries",
			f: func(client KolideService) error {
				_, _, err := client.RequestQueries(context.Background(), "node_key")
				return err
			},
		},
		{
			name: "PublishResults",
			f: func(client KolideService) error {
				_, _, _, err := client.PublishResults(context.Background(), "node_key", nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverResult := `{"disable_device": false}`

			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)

				resp := jsonrpc.Response{
					Result: []byte(serverResult),
				}

				respJson, err := json.Marshal(resp)
				require.NoError(t, err,
					"should be able to marshal test server response",
				)

				w.Write(respJson)
			}))

			u, err := url.Parse(testServer.URL)
			require.NoError(t, err,
				"should be able to parse test server URL",
			)

			mockKnapsack := mocks.NewKnapsack(t)
			mockKnapsack.On("KolideServerURL").Return(u.Host).Maybe()
			mockKnapsack.On("InsecureTransportTLS").Return(true).Maybe()
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger()).Maybe()
			mockKnapsack.On("InsecureTLS").Return(true).Maybe()
			mockKnapsack.On("CertPins").Return(nil).Maybe()

			clients := []KolideService{
				NewJSONRPCClient(mockKnapsack, nil),
				// TODO: test with grpc client
				// NewGRPCClient(mockKnapsack, nil),
			}

			for _, client := range clients {
				require.NoError(t, tt.f(client),
					"should not return an error when device is not disabled",
				)

				serverResult = `{"disable_device": true}`

				require.ErrorIs(t, tt.f(client), ErrDeviceDisabled{},
					"should return an ErrDeviceDisabled error when device is disabled",
				)
			}
		})
	}
}

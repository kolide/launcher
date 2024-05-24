package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/pb/launcher"
	"github.com/osquery/osquery-go/plugin/logger"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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

			// set up JSON-RPC test server
			jsonRpcServerResult := `{"disable_device": false}`

			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)

				resp := jsonrpc.Response{
					Result: []byte(jsonRpcServerResult),
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

			// set up GRPC test server
			grpcSvc := &mockDisableDeviceGrpcServer{}
			grpcServer := grpc.NewServer()

			RegisterGRPCServer(grpcServer, grpcSvc)

			listener, err := net.Listen("tcp", "localhost:0")
			require.NoError(t, err,
				"should be able to listen",
			)

			go func() {
				require.NoError(t, grpcServer.Serve(listener),
					"should be able to serve on a listener",
				)
			}()

			conn, err := grpc.Dial(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
			require.NoError(t, err,
				"should be able to dial a server",
			)

			// set up mock knapsack
			mockKnapsack := mocks.NewKnapsack(t)
			mockKnapsack.On("KolideServerURL").Return(u.Host)
			mockKnapsack.On("InsecureTransportTLS").Return(true)
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

			clients := []KolideService{
				NewJSONRPCClient(mockKnapsack, nil),
				NewGRPCClient(mockKnapsack, conn),
			}

			for _, client := range clients {
				jsonRpcServerResult = `{"disable_device": false}`
				grpcSvc.disableDevice = false

				require.NoError(t, tt.f(client),
					"should not return an error when device is not disabled",
				)

				jsonRpcServerResult = `{"disable_device": true}`
				grpcSvc.disableDevice = true

				require.ErrorIs(t, tt.f(client), ErrDeviceDisabled{},
					"should return an ErrDeviceDisabled error when device is disabled",
				)
			}
		})
	}
}

type mockDisableDeviceGrpcServer struct {
	launcher.UnimplementedApiServer
	disableDevice bool
}

func (m *mockDisableDeviceGrpcServer) PublishLogs(ctx context.Context, req *launcher.LogCollection) (*launcher.AgentApiResponse, error) {
	return &launcher.AgentApiResponse{
		DisableDevice: m.disableDevice,
	}, nil
}

func (m *mockDisableDeviceGrpcServer) PublishResults(ctx context.Context, req *launcher.ResultCollection) (*launcher.AgentApiResponse, error) {
	return &launcher.AgentApiResponse{
		DisableDevice: m.disableDevice,
	}, nil
}

func (m *mockDisableDeviceGrpcServer) RequestConfig(ctx context.Context, req *launcher.AgentApiRequest) (*launcher.ConfigResponse, error) {
	return &launcher.ConfigResponse{
		DisableDevice: m.disableDevice,
	}, nil
}

func (m *mockDisableDeviceGrpcServer) RequestEnrollment(ctx context.Context, req *launcher.EnrollmentRequest) (*launcher.EnrollmentResponse, error) {
	return &launcher.EnrollmentResponse{
		DisableDevice: m.disableDevice,
	}, nil
}

func (m *mockDisableDeviceGrpcServer) RequestQueries(ctx context.Context, req *launcher.AgentApiRequest) (*launcher.QueryCollection, error) {
	return &launcher.QueryCollection{
		DisableDevice: m.disableDevice,
	}, nil
}

package flareshipping

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/agent/types"
	agentTypesMocks "github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/debug/flareshipping/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRunFlareShip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		mockKnapsack           func(t *testing.T) types.Knapsack
		expectSignatureHeaders bool
		expectSecret           bool
		assertion              assert.ErrorAssertionFunc
	}{
		{
			name:         "happy path no signing keys",
			mockKnapsack: func(t *testing.T) types.Knapsack { return nil },
			assertion:    assert.NoError,
			// if the secret exists at the default path, is should error when
			// we try to read it unless we run tests as root
			expectSecret: false,
		},
		{
			name: "happy path with signing keys",
			mockKnapsack: func(t *testing.T) types.Knapsack {
				configStore := inmemory.NewStore(log.NewNopLogger())
				k := agentTypesMocks.NewKnapsack(t)
				k.On("ConfigStore").Return(configStore)
				k.On("EnrollSecret").Return("enroll_secret_value")
				return k
			},
			expectSignatureHeaders: true,
			expectSecret:           true,
			assertion:              assert.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			flarer := mocks.NewFlarer(t)
			flarer.On("RunFlare", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			testServer := httptest.NewServer(nil)

			mux := http.NewServeMux()
			mux.Handle("/signedurl", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()

				for _, header := range []string{control.HeaderKey, control.HeaderSignature} {
					_, ok := r.Header[header]
					assert.Equal(t, tt.expectSignatureHeaders, ok)
				}

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				data := make(map[string]string, 3)
				require.NoError(t, json.Unmarshal(body, &data))

				require.Equal(t, tt.expectSecret, len(data["enroll_secret"]) > 0)
				require.NotEmpty(t, data["hostname"])
				require.NotEmpty(t, data["username"])

				w.Write([]byte(fmt.Sprintf("%s/upload", testServer.URL)))
			}))

			mux.Handle("/upload", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

			testServer.Config.Handler = mux

			tt.assertion(t, RunFlareShip(log.NewNopLogger(), tt.mockKnapsack(t), flarer, fmt.Sprintf("%s/signedurl", testServer.URL)))
		})
	}
}

func TestRunFlareShipErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		failGetSignedURLRequest bool
		failUploadRequest       bool
	}{
		{
			name:                    "fail get signed url request",
			failGetSignedURLRequest: true,
		},
		{
			name:              "fail upload request",
			failUploadRequest: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			flarer := mocks.NewFlarer(t)

			if !tt.failGetSignedURLRequest {
				flarer.On("RunFlare", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			}

			testServer := httptest.NewServer(nil)

			mux := http.NewServeMux()
			mux.Handle("/signedurl", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()

				if tt.failGetSignedURLRequest {
					w.WriteHeader(http.StatusInternalServerError)
				}

				w.Write([]byte(fmt.Sprintf("%s/upload", testServer.URL)))
			}))

			mux.Handle("/upload", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.failUploadRequest {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))

			testServer.Config.Handler = mux

			require.Error(t, RunFlareShip(log.NewNopLogger(), nil, flarer, fmt.Sprintf("%s/signedurl", testServer.URL)))
		})
	}
}

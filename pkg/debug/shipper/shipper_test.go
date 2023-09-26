package shipper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	typesMocks "github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		mockKnapsack           func(t *testing.T) *typesMocks.Knapsack
		expectSignatureHeaders bool
		expectSecret           bool
		assertion              assert.ErrorAssertionFunc
	}{
		{
			name: "happy path no signing keys",
			mockKnapsack: func(t *testing.T) *typesMocks.Knapsack {
				k := typesMocks.NewKnapsack(t)
				k.On("EnrollSecret").Return("")
				return k
			},
			assertion: assert.NoError,
			// if the secret exists at the default path, is should error when
			// we try to read it unless we run tests as root
			expectSecret: false,
		},
		{
			name: "happy path with signing keys",
			mockKnapsack: func(t *testing.T) *typesMocks.Knapsack {
				configStore := inmemory.NewStore(log.NewNopLogger())
				agent.SetupKeys(log.NewNopLogger(), configStore)

				k := typesMocks.NewKnapsack(t)
				k.On("EnrollSecret").Return("enroll_secret_value")
				return k
			},
			expectSignatureHeaders: true,
			expectSecret:           true,
			assertion:              assert.NoError,
		},
	}
	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
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

			knapsack := tt.mockKnapsack(t)
			knapsack.On("DebugUploadRequestURL").Return(fmt.Sprintf("%s/signedurl", testServer.URL))

			tt.assertion(t, ship(log.NewNopLogger(), knapsack, "some note", bytes.NewBuffer([]byte("ahhhhh"))))
		})
	}
}

func TestShipErrors(t *testing.T) {
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

			k := typesMocks.NewKnapsack(t)
			k.On("DebugUploadRequestURL").Return(fmt.Sprintf("%s/signedurl", testServer.URL))
			k.On("EnrollSecret").Return("")

			require.Error(t, ship(log.NewNopLogger(), k, "some note", bytes.NewBuffer([]byte("ahhhhh"))))
		})
	}
}

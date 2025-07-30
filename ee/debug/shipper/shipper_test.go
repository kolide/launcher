package shipper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShip(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		name                   string
		mockKnapsack           func(t *testing.T) *typesMocks.Knapsack
		expectSignatureHeaders bool
		expectSecret           bool
		expectMunemo           bool
		withUploadRequestURL   bool
		assertion              assert.ErrorAssertionFunc
	}{
		{
			name: "no enroll secret, upload request url opt",
			mockKnapsack: func(t *testing.T) *typesMocks.Knapsack {
				k := typesMocks.NewKnapsack(t)
				k.On("EnrollSecret").Return("")
				k.On("EnrollSecretPath").Return("")
				k.On("Slogger").Return(multislogger.NewNopLogger())
				k.On("Registrations").Return([]types.Registration{
					{
						RegistrationID: types.DefaultRegistrationID,
						Munemo:         "test-munemo",
					},
				}, nil)
				return k
			},
			assertion:            assert.NoError,
			withUploadRequestURL: true,
			// if the secret exists at the default path, is should error when
			// we try to read it unless we run tests as root
			expectSecret: false,
			expectMunemo: true,
		},
		{
			name: "no enroll secret, signed url req from knapsack",
			mockKnapsack: func(t *testing.T) *typesMocks.Knapsack {
				k := typesMocks.NewKnapsack(t)
				k.On("EnrollSecret").Return("")
				k.On("EnrollSecretPath").Return("")
				k.On("Slogger").Return(multislogger.NewNopLogger())
				k.On("Registrations").Return([]types.Registration{}, nil)
				k.On("RootDirectory").Return(t.TempDir())
				return k
			},
			assertion:            assert.NoError,
			withUploadRequestURL: false,
			// if the secret exists at the default path, is should error when
			// we try to read it unless we run tests as root
			expectSecret: false,
			expectMunemo: false,
		},
		{
			name: "happy path with signing keys and enroll secret",
			mockKnapsack: func(t *testing.T) *typesMocks.Knapsack {
				configStore := inmemory.NewStore()
				agent.SetupKeys(context.TODO(), multislogger.NewNopLogger(), configStore)

				k := typesMocks.NewKnapsack(t)
				k.On("EnrollSecret").Return("enroll_secret_value")
				k.On("Slogger").Return(multislogger.NewNopLogger())
				k.On("Registrations").Return([]types.Registration{
					{
						RegistrationID: types.DefaultRegistrationID,
						Munemo:         "test-munemo",
					},
				}, nil)
				return k
			},
			expectSignatureHeaders: true,
			expectSecret:           true,
			expectMunemo:           true,
			withUploadRequestURL:   true,
			assertion:              assert.NoError,
		},
	}
	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			testServer := httptest.NewServer(nil)

			mux := http.NewServeMux()
			mux.Handle("/api/agent/flare", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				require.Equal(t, tt.expectMunemo, len(data["munemo"]) > 0)
				require.NotEmpty(t, data["hostname"])
				require.NotEmpty(t, data["note"])
				require.NotEmpty(t, data["console_users"])
				require.NotEmpty(t, data["running_user"])
				urlData := struct {
					URL string
				}{
					URL: fmt.Sprintf("%s/upload", testServer.URL),
				}

				urlDataJson, err := json.Marshal(urlData)
				require.NoError(t, err)

				w.Write(urlDataJson)
			}))

			uploadBody := []byte("some_data")

			mux.Handle("/upload", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, uploadBody, body)
			}))

			testServer.Config.Handler = mux

			knapsack := tt.mockKnapsack(t)

			var shipper *shipper
			var err error
			if tt.withUploadRequestURL {
				shipper, err = New(knapsack, WithNote("woo"), WithUploadRequestURL(fmt.Sprintf("%s/api/agent/flare", testServer.URL)))
			} else {
				knapsack.On("KolideServerURL").Return(testServer.URL)
				shipper, err = New(knapsack, WithNote("woo"))
			}

			require.NoError(t, err)

			_, err = shipper.Write(uploadBody)
			require.NoError(t, err)
			require.NoError(t, shipper.Close())
		})
	}
}

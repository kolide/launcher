package shipper

import (
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

func TestShip(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		name                   string
		mockKnapsack           func(t *testing.T) *typesMocks.Knapsack
		expectSignatureHeaders bool
		expectSecret           bool
		withUploadUrl          bool
		withUploadRequestURL   bool
		assertion              assert.ErrorAssertionFunc
	}{
		{
			name: "happy path no enroll secret",
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
			name: "happy path with signing keys and enroll secret",
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
		{
			name: "happy path with provided upload url",
			mockKnapsack: func(t *testing.T) *typesMocks.Knapsack {
				k := typesMocks.NewKnapsack(t)
				return k
			},
			expectSignatureHeaders: true,
			expectSecret:           true,
			assertion:              assert.NoError,
			withUploadUrl:          true,
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
			if tt.withUploadUrl {
				shipper, err = New(knapsack, WithUploadURL(fmt.Sprintf("%s/upload", testServer.URL)))
				require.NoError(t, err)
			} else {
				shipper, err = New(knapsack, WithUploadRequestURL(fmt.Sprintf("%s/signedurl", testServer.URL)))
			}
			require.NoError(t, err)

			_, err = shipper.Write(uploadBody)
			require.NoError(t, err)
			require.NoError(t, shipper.Close())
		})
	}
}

func TestShipErrors(t *testing.T) { //nolint:paralleltest
	type errorType int

	const (
		failedToGetSignedURL errorType = iota
		failedToUpload
		noUploadOrUploadReqeustURL
	)

	tests := []struct {
		name      string
		errorType errorType
	}{
		{
			name:      "fail get signed url request",
			errorType: failedToGetSignedURL,
		},
		{
			name:      "bad upload url",
			errorType: failedToUpload,
		},
		{
			name:      "no upload or upload reqeust URL option",
			errorType: noUploadOrUploadReqeustURL,
		},
	}
	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testServer := httptest.NewServer(nil)

			mux := http.NewServeMux()
			mux.Handle("/signedurl", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				w.WriteHeader(http.StatusInternalServerError)
			}))

			testServer.Config.Handler = mux

			k := typesMocks.NewKnapsack(t)
			var shipper *shipper
			var err error

			switch tt.errorType {
			case failedToGetSignedURL:
				// should error on New() since /signedurl will return 500
				k.On("EnrollSecret").Return("")
				shipper, err = New(k, WithUploadRequestURL(fmt.Sprintf("%s/signedurl", testServer.URL)))
				require.Error(t, err)

			case failedToUpload:
				// should error on Close() since /bad_endpoint doesnt exist
				shipper, err = New(k, WithUploadURL(fmt.Sprintf("%s/bad_endpoint", testServer.URL)))
				require.NoError(t, err)
				_, err = shipper.Write([]byte("some_data"))
				require.NoError(t, err)
				require.Error(t, shipper.Close())

			case noUploadOrUploadReqeustURL:
				// should error on New() since no upload or upload request url
				shipper, err = New(k)
				require.Error(t, err)

			default:
				t.Fail()
			}
		})
	}
}

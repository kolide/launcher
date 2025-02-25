package localserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_requestZtaInfoHandler(t *testing.T) {
	t.Parallel()

	// Set up our ZTA store with some test data in it
	slogger := multislogger.NewNopLogger()
	ztaInfoStore, err := storageci.NewStore(t, slogger, storage.ZtaInfoStore.String())
	require.NoError(t, err)
	testZtaInfo, err := json.Marshal(map[string]string{
		"some_test_data": "some_test_value",
	})
	require.NoError(t, err)
	require.NoError(t, ztaInfoStore.Set(localserverZtaInfoKey, testZtaInfo))

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("ZtaInfoStore").Return(ztaInfoStore)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/zta", nil)
	request.Header.Set("origin", acceptableOrigin(t))
	responseRecorder := httptest.NewRecorder()
	ls.requestZtaInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was successful and contains the data we expect
	require.Equal(t, http.StatusOK, responseRecorder.Code)
	require.Equal(t, "application/json", responseRecorder.Header().Get("Content-Type"))
	require.Equal(t, testZtaInfo, responseRecorder.Body.Bytes())

	k.AssertExpectations(t)
}

func acceptableOrigin(t *testing.T) string {
	// Just grab the first origin available in our allowlist
	acceptableOrigin := ""
	for k := range allowlistedZtaOriginsLookup {
		acceptableOrigin = k
		break
	}
	if acceptableOrigin == "" {
		t.Error("no acceptable origins found")
		t.FailNow()
	}

	return acceptableOrigin
}

func Test_requestZtaInfoHandler_allowsAllSafariWebExtensionOrigins(t *testing.T) {
	t.Parallel()

	// Set up our ZTA store with some test data in it
	slogger := multislogger.NewNopLogger()
	ztaInfoStore, err := storageci.NewStore(t, slogger, storage.ZtaInfoStore.String())
	require.NoError(t, err)
	testZtaInfo, err := json.Marshal(map[string]string{
		"some_test_data": "some_test_value",
	})
	require.NoError(t, err)
	require.NoError(t, ztaInfoStore.Set(localserverZtaInfoKey, testZtaInfo))

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("ZtaInfoStore").Return(ztaInfoStore)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/zta", nil)
	request.Header.Set("origin", fmt.Sprintf("%sexample.com", safariWebExtensionScheme))
	responseRecorder := httptest.NewRecorder()
	ls.requestZtaInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was successful and contains the data we expect
	require.Equal(t, http.StatusOK, responseRecorder.Code)
	require.Equal(t, "application/json", responseRecorder.Header().Get("Content-Type"))
	require.Equal(t, testZtaInfo, responseRecorder.Body.Bytes())

	k.AssertExpectations(t)
}

func Test_requestZtaInfoHandler_badRequest(t *testing.T) {
	t.Parallel()

	reqOrigin := acceptableOrigin(t)

	for _, tt := range []struct {
		testCaseName           string
		httpMethod             string
		requestOrigin          string
		requestBody            io.Reader
		expectedResponseStatus int
	}{
		{
			testCaseName:           http.MethodPost,
			httpMethod:             http.MethodPost,
			requestOrigin:          reqOrigin,
			requestBody:            http.NoBody,
			expectedResponseStatus: http.StatusMethodNotAllowed,
		},
		{
			testCaseName:           http.MethodPut,
			httpMethod:             http.MethodPut,
			requestOrigin:          reqOrigin,
			requestBody:            http.NoBody,
			expectedResponseStatus: http.StatusMethodNotAllowed,
		},
		{
			testCaseName:           http.MethodPatch,
			httpMethod:             http.MethodPatch,
			requestOrigin:          reqOrigin,
			requestBody:            http.NoBody,
			expectedResponseStatus: http.StatusMethodNotAllowed,
		},
		{
			testCaseName:           http.MethodDelete,
			httpMethod:             http.MethodDelete,
			requestOrigin:          reqOrigin,
			requestBody:            http.NoBody,
			expectedResponseStatus: http.StatusMethodNotAllowed,
		},
		{
			testCaseName:           "disallowed origin",
			httpMethod:             http.MethodGet,
			requestOrigin:          "https://example.com",
			requestBody:            http.NoBody,
			expectedResponseStatus: http.StatusForbidden,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our localserver dependencies
			slogger := multislogger.NewNopLogger()
			k := typesmocks.NewKnapsack(t)
			k.On("KolideServerURL").Return("localserver")
			k.On("Slogger").Return(slogger)

			// Set up localserver
			ls, err := New(context.TODO(), k, nil)
			require.NoError(t, err)

			// Make a request to our handler
			request := httptest.NewRequest(tt.httpMethod, "/zta", tt.requestBody)
			request.Header.Set("origin", tt.requestOrigin)
			responseRecorder := httptest.NewRecorder()
			ls.requestZtaInfoHandler().ServeHTTP(responseRecorder, request)

			// Make sure we got back an expected response status code (4xx-level)
			require.Equal(t, tt.expectedResponseStatus, responseRecorder.Code)

			k.AssertExpectations(t)
		})
	}
}

func Test_requestZtaInfoHandler_noDataAvailable(t *testing.T) {
	t.Parallel()

	// Set up our ZTA store, but do not store any data in it under the `localserverZtaInfoKey` key
	slogger := multislogger.NewNopLogger()
	ztaInfoStore, err := storageci.NewStore(t, slogger, storage.ZtaInfoStore.String())
	require.NoError(t, err)

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("ZtaInfoStore").Return(ztaInfoStore)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/zta", nil)
	request.Header.Set("origin", acceptableOrigin(t))
	responseRecorder := httptest.NewRecorder()
	ls.requestZtaInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was a 404
	require.Equal(t, http.StatusNotFound, responseRecorder.Code)

	k.AssertExpectations(t)
}

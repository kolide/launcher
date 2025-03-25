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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_requestDt4aInfoHandler(t *testing.T) {
	t.Parallel()

	// Set up our dt4a store with some test data in it
	slogger := multislogger.NewNopLogger()
	dt4aInfoStore, err := storageci.NewStore(t, slogger, storage.Dt4aInfoStore.String())
	require.NoError(t, err)
	testDt4aInfo, err := json.Marshal(map[string]string{
		"some_test_data": "some_test_value",
	})
	require.NoError(t, err)
	require.NoError(t, dt4aInfoStore.Set(localserverDt4aInfoKey, testDt4aInfo))

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("Dt4aInfoStore").Return(dt4aInfoStore)
	k.On("AllowOverlyBroadDt4aAcceleration").Return(false)
	k.On("ReadEnrollSecret").Return("enroll_secret", nil)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/dt4a", nil)
	request.Header.Set("origin", acceptableOrigin(t))
	responseRecorder := httptest.NewRecorder()
	ls.requestDt4aInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was successful and contains the data we expect
	require.Equal(t, http.StatusOK, responseRecorder.Code)
	require.Equal(t, "application/json", responseRecorder.Header().Get("Content-Type"))
	require.Equal(t, testDt4aInfo, responseRecorder.Body.Bytes())

	k.AssertExpectations(t)
}

func acceptableOrigin(t *testing.T) string {
	// Just grab the first origin available in our allowlist
	acceptableOrigin := ""
	for k := range allowlistedDt4aOriginsLookup {
		acceptableOrigin = k
		break
	}
	if acceptableOrigin == "" {
		t.Error("no acceptable origins found")
		t.FailNow()
	}

	return acceptableOrigin
}

func Test_requestDt4aInfoHandler_allowsAllSafariWebExtensionOrigins(t *testing.T) {
	t.Parallel()

	// Set up our dt4a store with some test data in it
	slogger := multislogger.NewNopLogger()
	dt4aInfoStore, err := storageci.NewStore(t, slogger, storage.Dt4aInfoStore.String())
	require.NoError(t, err)
	testDt4aInfo, err := json.Marshal(map[string]string{
		"some_test_data": "some_test_value",
	})
	require.NoError(t, err)
	require.NoError(t, dt4aInfoStore.Set(localserverDt4aInfoKey, testDt4aInfo))

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("Dt4aInfoStore").Return(dt4aInfoStore)
	k.On("AllowOverlyBroadDt4aAcceleration").Return(false)
	k.On("ReadEnrollSecret").Return("enroll_secret", nil)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/dt4a", nil)
	request.Header.Set("origin", fmt.Sprintf("%sexample.com", safariWebExtensionScheme))
	responseRecorder := httptest.NewRecorder()
	ls.requestDt4aInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was successful and contains the data we expect
	require.Equal(t, http.StatusOK, responseRecorder.Code)
	require.Equal(t, "application/json", responseRecorder.Header().Get("Content-Type"))
	require.Equal(t, testDt4aInfo, responseRecorder.Body.Bytes())

	k.AssertExpectations(t)
}

func Test_requestDt4aInfoHandler_allowsMissingOrigin(t *testing.T) {
	t.Parallel()

	// Set up our dt4a store with some test data in it
	slogger := multislogger.NewNopLogger()
	dt4aInfoStore, err := storageci.NewStore(t, slogger, storage.Dt4aInfoStore.String())
	require.NoError(t, err)
	testDt4aInfo, err := json.Marshal(map[string]string{
		"some_test_data": "some_test_value",
	})
	require.NoError(t, err)
	require.NoError(t, dt4aInfoStore.Set(localserverDt4aInfoKey, testDt4aInfo))

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("Dt4aInfoStore").Return(dt4aInfoStore)
	k.On("AllowOverlyBroadDt4aAcceleration").Return(false)
	k.On("ReadEnrollSecret").Return("enroll_secret", nil)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/dt4a", nil)
	responseRecorder := httptest.NewRecorder()
	ls.requestDt4aInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was successful and contains the data we expect
	require.Equal(t, http.StatusOK, responseRecorder.Code)
	require.Equal(t, "application/json", responseRecorder.Header().Get("Content-Type"))
	require.Equal(t, testDt4aInfo, responseRecorder.Body.Bytes())

	k.AssertExpectations(t)
}

func Test_requestDt4aInfoHandler_allowsEmptyOrigin(t *testing.T) {
	t.Parallel()

	// Set up our dt4a store with some test data in it
	slogger := multislogger.NewNopLogger()
	dt4aInfoStore, err := storageci.NewStore(t, slogger, storage.Dt4aInfoStore.String())
	require.NoError(t, err)
	testDt4aInfo, err := json.Marshal(map[string]string{
		"some_test_data": "some_test_value",
	})
	require.NoError(t, err)
	require.NoError(t, dt4aInfoStore.Set(localserverDt4aInfoKey, testDt4aInfo))

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("Dt4aInfoStore").Return(dt4aInfoStore)
	k.On("AllowOverlyBroadDt4aAcceleration").Return(false)
	k.On("ReadEnrollSecret").Return("enroll_secret", nil)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/dt4a", nil)
	request.Header.Set("origin", "")
	responseRecorder := httptest.NewRecorder()
	ls.requestDt4aInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was successful and contains the data we expect
	require.Equal(t, http.StatusOK, responseRecorder.Code)
	require.Equal(t, "application/json", responseRecorder.Header().Get("Content-Type"))
	require.Equal(t, testDt4aInfo, responseRecorder.Body.Bytes())

	k.AssertExpectations(t)
}

func Test_requestDt4aInfoHandler_badRequest(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName           string
		httpMethod             string
		requestOrigin          string
		requestBody            io.Reader
		expectedResponseStatus int
	}{
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
			k.On("AllowOverlyBroadDt4aAcceleration").Maybe().Return(false)
			k.On("ReadEnrollSecret").Return("enroll_secret", nil)

			// Set up localserver
			ls, err := New(context.TODO(), k, nil)
			require.NoError(t, err)

			// Make a request to our handler
			request := httptest.NewRequest(tt.httpMethod, "/dt4a", tt.requestBody)
			request.Header.Set("origin", tt.requestOrigin)
			responseRecorder := httptest.NewRecorder()
			ls.requestDt4aInfoHandler().ServeHTTP(responseRecorder, request)

			// Make sure we got back an expected response status code (4xx-level)
			require.Equal(t, tt.expectedResponseStatus, responseRecorder.Code)

			k.AssertExpectations(t)
		})
	}
}

func Test_requestDt4aInfoHandler_noDataAvailable(t *testing.T) {
	t.Parallel()

	// Set up our dt4a store, but do not store any data in it under the `localserverDt4aInfoKey` key
	slogger := multislogger.NewNopLogger()
	dt4aInfoStore, err := storageci.NewStore(t, slogger, storage.Dt4aInfoStore.String())
	require.NoError(t, err)

	// Set up the rest of our localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(slogger)
	k.On("Dt4aInfoStore").Return(dt4aInfoStore)
	k.On("AllowOverlyBroadDt4aAcceleration").Return(false)
	k.On("ReadEnrollSecret").Return("enroll_secret", nil)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/dt4a", nil)
	request.Header.Set("origin", acceptableOrigin(t))
	responseRecorder := httptest.NewRecorder()
	ls.requestDt4aInfoHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was a 404
	require.Equal(t, http.StatusNotFound, responseRecorder.Code)

	k.AssertExpectations(t)
}

func Test_requestDt4aAccelerationHandler(t *testing.T) {
	t.Parallel()

	// Set up localserver dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("Slogger").Return(multislogger.NewNopLogger())
	// Validate that we accelerate control requests
	k.On("SetControlRequestIntervalOverride", mock.Anything, mock.Anything).Return()
	// Validate that we accelerate osquery distributed requests
	k.On("SetDistributedForwardingIntervalOverride", mock.Anything, mock.Anything).Return()
	k.On("ReadEnrollSecret").Return("enroll_secret", nil)

	// Set up localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Make a request to our handler
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("origin", acceptableOrigin(t))
	responseRecorder := httptest.NewRecorder()
	ls.requestDt4aAccelerationHandler().ServeHTTP(responseRecorder, request)

	// Make sure response was successful and contains the data we expect
	require.Equal(t, http.StatusNoContent, responseRecorder.Code)

	k.AssertExpectations(t)
}

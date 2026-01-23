package localserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/osquerypublisher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestIdHandler(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesMocks.NewKnapsack(t)
	mockKnapsack.On("KolideServerURL").Return("localhost")
	mockKnapsack.On("CurrentEnrollmentStatus").Return(types.Enrolled, nil)
	mockKnapsack.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil)
	mockKnapsack.On("Registrations").Return([]types.Registration{
		{
			RegistrationID: types.DefaultEnrollmentID,
			Munemo:         "test-munemo",
		},
	}, nil)
	testConfigStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err, "could not create test config store")
	mockKnapsack.On("ConfigStore").Return(testConfigStore).Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	mockKnapsack.On("TokenStore").Return(tokenStore)
	osqPublisher := osquerypublisher.NewLogPublisherClient(multislogger.NewNopLogger(), mockKnapsack, http.DefaultClient)
	mockKnapsack.On("OsqueryPublisher").Return(osqPublisher)

	var logBytes bytes.Buffer
	slogger := slog.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack.On("Slogger").Return(slogger)

	server := testServer(t, mockKnapsack)

	req, err := http.NewRequestWithContext(t.Context(), "", "", nil) //nolint:noctx // Don't care about this in tests
	require.NoError(t, err)

	handler := http.HandlerFunc(server.requestIdHandlerFunc)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// We want to test that there were no errors or warnings logged. We do this indiredtly, by making sure the log
	// only contains what we expect. There's probably a cleaner way....
	// Right now, we expect a single log line about certificates
	assert.Equal(t, 1, strings.Count(logBytes.String(), "\n"))
	assert.Contains(t, logBytes.String(), "certificate")

	assert.Equal(t, http.StatusOK, rr.Code)

	// convert the response to a struct
	var response requestIdsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))

	mockKnapsack.AssertExpectations(t)
}

func testServer(t *testing.T, k types.Knapsack) *localServer {
	server, err := New(t.Context(), k, nil)
	require.NoError(t, err)
	return server
}

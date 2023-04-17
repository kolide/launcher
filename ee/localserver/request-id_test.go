package localserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	typesMocks "github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestIdHandler(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesMocks.NewKnapsack(t)
	mockKnapsack.On("ConfigStore").Return(storageci.NewStore(t, log.NewNopLogger(), storage.ConfigStore.String()))

	var logBytes bytes.Buffer
	server := testServer(t, mockKnapsack, &logBytes)

	req, err := http.NewRequest("", "", nil)
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

	// in the current CI environment (GitHub Actions) the linux runner
	// does not have a console user, so we expect an empty list
	if os.Getenv("CI") == "true" && runtime.GOOS == "linux" {
		assert.Empty(t, response.ConsoleUsers)
		return
	}

	assert.GreaterOrEqual(t, len(response.ConsoleUsers), 1, "should have at least one console user")
}

func testServer(t *testing.T, k types.Knapsack, logBytes *bytes.Buffer) *localServer {
	require.NoError(t, osquery.SetupLauncherKeys(k.ConfigStore()))

	server, err := New(k, "", WithLogger(log.NewLogfmtLogger(logBytes)))
	require.NoError(t, err)
	return server
}

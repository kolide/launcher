package localserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/go-kit/kit/log"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestIdHandler(t *testing.T) {
	t.Parallel()

	var logBytes bytes.Buffer
	server := testServer(t, &logBytes)

	req, err := http.NewRequest("", "", nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(server.requestIdHandlerFunc)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Empty(t, logBytes.String())
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

func testServer(t *testing.T, logBytes *bytes.Buffer) *localServer {
	s, err := storageci.NewStore(t, log.NewNopLogger(), osquery.ConfigBucket)
	require.NoError(t, err)

	require.NoError(t, osquery.SetupLauncherKeys(s))

	server, err := New(log.NewLogfmtLogger(logBytes), s, "")
	require.NoError(t, err)
	return server
}

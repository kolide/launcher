package localserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
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

	db, err := bbolt.Open(filepath.Join(t.TempDir(), "local_server_test.db"), 0600, &bbolt.Options{
		Timeout: 1 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, osquery.SetupLauncherKeys(db))

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	server, err := New(log.NewLogfmtLogger(logBytes), db, "")
	require.NoError(t, err)
	return server
}

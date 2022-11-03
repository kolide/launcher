package debug

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getDebugURL(t *testing.T, tokenPath string) string {
	url, err := ioutil.ReadFile(tokenPath)
	require.Nil(t, err)
	return string(url)
}

func TestStartDebugServer(t *testing.T) {
	t.Parallel()
	tokenFile, err := ioutil.TempFile("", "kolide_debug_test")
	require.Nil(t, err)

	serv, _, err := startDebugServer(tokenFile.Name(), log.NewNopLogger())
	require.Nil(t, err)

	url := getDebugURL(t, tokenFile.Name())
	resp, err := http.Get(url)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	err = serv.Shutdown(context.Background())
	require.Nil(t, err)
}

func TestDebugServerUnauthorized(t *testing.T) {
	t.Parallel()
	tokenFile, err := ioutil.TempFile("", "kolide_debug_test")
	require.Nil(t, err)

	serv, _, err := startDebugServer(tokenFile.Name(), log.NewNopLogger())
	require.Nil(t, err)

	url := getDebugURL(t, tokenFile.Name())
	resp, err := http.Get(url + "bad_token")
	require.Nil(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	err = serv.Shutdown(context.Background())
	require.Nil(t, err)
}

func TestAttachDebugHandler(t *testing.T) {
	t.Parallel()

	rootDir := testRootDir(t)
	AttachDebugHandler(rootDir, log.NewNopLogger())

	// Start server
	url, err := ToggleDebugServer(rootDir)
	require.NoError(t, err)

	resp, err := http.Get(url)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	// Stop server
	_, err = ToggleDebugServer(rootDir)
	require.NoError(t, err)

	_, err = http.Get(url)
	require.Error(t, err)

	// Start server
	url, err = ToggleDebugServer(rootDir)
	require.NoError(t, err)

	resp, err = http.Get(url)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Stop server
	_, err = ToggleDebugServer(rootDir)
	require.NoError(t, err)

	_, err = http.Get(url)
	require.Error(t, err)
}

func testRootDir(t *testing.T) string {
	// on windows were not worried about socket length since it's all based on the named pipe convention
	if runtime.GOOS == "windows" {
		return t.TempDir()
	}

	// keeping it short so we don't hit the 103 char limit for unix sockets
	rootDir := filepath.Join("tmp", ulid.New())
	require.NoError(t, os.MkdirAll(rootDir, 0700))

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(rootDir))
	})

	return rootDir
}

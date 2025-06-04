//go:build !windows
// +build !windows

//nolint:bodyclose
package debug

import (
	"context"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getDebugURL(t *testing.T, tokenPath string) string {
	url, err := os.ReadFile(tokenPath)
	require.Nil(t, err)
	return string(url)
}

func TestStartDebugServer(t *testing.T) {
	t.Parallel()
	tokenFile, err := os.CreateTemp(t.TempDir(), "kolide_debug_test")
	require.Nil(t, err)
	t.Cleanup(func() {
		tokenFile.Close()
	})

	serv, err := startDebugServer(tokenFile.Name(), multislogger.NewNopLogger())
	require.Nil(t, err)

	url := getDebugURL(t, tokenFile.Name())
	resp, err := http.Get(url) //nolint:noctx // We don't care about this in tests
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	err = serv.Shutdown(context.Background())
	require.Nil(t, err)
}

func TestDebugServerUnauthorized(t *testing.T) {
	t.Parallel()
	tokenFile, err := os.CreateTemp(t.TempDir(), "kolide_debug_test")
	require.Nil(t, err)
	t.Cleanup(func() {
		tokenFile.Close()
	})

	serv, err := startDebugServer(tokenFile.Name(), multislogger.NewNopLogger())
	require.Nil(t, err)

	url := getDebugURL(t, tokenFile.Name())
	resp, err := http.Get(url + "bad_token") //nolint:noctx // We don't care about this in tests
	require.Nil(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	err = serv.Shutdown(context.Background())
	require.Nil(t, err)
}

func TestAttachDebugHandler(t *testing.T) {
	t.Parallel()

	tokenFile, err := os.CreateTemp(t.TempDir(), "kolide_debug_test")
	require.Nil(t, err)
	t.Cleanup(func() {
		tokenFile.Close()
	})

	AttachDebugHandler(tokenFile.Name(), multislogger.NewNopLogger())

	// Start server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	url := getDebugURL(t, tokenFile.Name())
	resp, err := http.Get(url) //nolint:noctx // We don't care about this in tests
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	// Stop server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	_, err = http.Get(url) //nolint:noctx // We don't care about this in tests
	require.NotNil(t, err)

	// Start server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	newUrl := getDebugURL(t, tokenFile.Name())
	assert.NotEqual(t, url, newUrl)

	_, err = http.Get(newUrl) //nolint:noctx // We don't care about this in tests
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Stop server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	_, err = http.Get(url) //nolint:noctx // We don't care about this in tests
	require.NotNil(t, err)
}

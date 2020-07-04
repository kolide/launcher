package debug

import (
	"context"
	"io/ioutil"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
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

	serv, err := startDebugServer(tokenFile.Name(), log.NewNopLogger())
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

	serv, err := startDebugServer(tokenFile.Name(), log.NewNopLogger())
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
	t.Skip("TODO: Windows tests")

	tokenFile, err := ioutil.TempFile("", "kolide_debug_test")
	require.Nil(t, err)

	AttachDebugHandler(tokenFile.Name(), log.NewNopLogger())

	// Start server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	url := getDebugURL(t, tokenFile.Name())
	resp, err := http.Get(url)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Stop server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	resp, err = http.Get(url)
	require.NotNil(t, err)

	// Start server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	newUrl := getDebugURL(t, tokenFile.Name())
	assert.NotEqual(t, url, newUrl)

	resp, err = http.Get(newUrl)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Stop server
	syscall.Kill(syscall.Getpid(), debugSignal)
	time.Sleep(1 * time.Second)

	resp, err = http.Get(url)
	require.NotNil(t, err)
}

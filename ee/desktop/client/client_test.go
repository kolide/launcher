package client

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"

	"github.com/kolide/launcher/ee/desktop/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Shutdown(t *testing.T) {
	t.Parallel()

	const validAuthToken = "test-auth-header"
	tests := []struct {
		name string
	}{
		{
			name: "happy_path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			socketPath := testSocketPath(t)
			shutdownChan := make(chan struct{})
			server, err := server.New(log.NewNopLogger(), validAuthToken, socketPath, shutdownChan)
			require.NoError(t, err)

			go func() {
				server.Serve()
			}()

			go func() {
				<-shutdownChan
			}()

			client := New(validAuthToken, socketPath)
			err = client.Shutdown()
			assert.NoError(t, err)
		})
	}
}

func testSocketPath(t *testing.T) string {
	socketFileName := strings.Replace(t.Name(), "/", "_", -1)

	// using t.TempDir() creates a file path too long for a unix socket
	socketPath := filepath.Join(os.TempDir(), socketFileName)
	// truncate socket path to max length
	if len(socketPath) > 103 {
		socketPath = socketPath[:103]
	}

	if runtime.GOOS == "windows" {
		socketPath = fmt.Sprintf(`\\.\pipe\%s`, socketFileName)
	}

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(socketPath))
	})

	return socketPath
}

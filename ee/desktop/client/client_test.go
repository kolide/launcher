package client

import (
	"context"
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

func TestClient_GetAndShutdown(t *testing.T) {
	t.Parallel()

	const validAuthToken = "test-auth-header"
	tests := []struct {
		name          string
		paths         []string
		expectedError bool
	}{
		{
			name:          "valid_paths",
			paths:         []string{"ping", "shutdown", "refresh"},
			expectedError: false,
		},
		{
			name:          "invalid_paths",
			paths:         []string{"", "never-a-real-path"},
			expectedError: true,
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
			for _, path := range tt.paths {
				err := client.get(path)

				if tt.expectedError {
					require.Error(t, err)
					continue
				}

				require.NoError(t, err)
			}
			assert.NoError(t, server.Shutdown(context.Background()))
		})
	}
}

func testSocketPath(t *testing.T) string {
	socketFileName := strings.Replace(t.Name(), "/", "_", -1)

	// using t.TempDir() creates a file path too long for a unix socket
	socketPath := filepath.Join(os.TempDir(), socketFileName)
	// truncate socket path to max length
	const maxSocketLength = 103
	if len(socketPath) > maxSocketLength {
		socketPath = socketPath[:maxSocketLength]
	}

	if runtime.GOOS == "windows" {
		socketPath = fmt.Sprintf(`\\.\pipe\%s`, socketFileName)
	}

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(socketPath))
	})

	return socketPath
}

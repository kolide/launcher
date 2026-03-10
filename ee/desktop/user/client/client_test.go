package client

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kolide/launcher/v2/ee/desktop/user/server"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			socketPath := testSocketPath(t)
			shutdownChan := make(chan struct{})
			server, err := server.New(multislogger.NewNopLogger(), validAuthToken, socketPath, shutdownChan, make(chan<- struct{}), nil)
			require.NoError(t, err)

			// Start server
			go func() {
				server.Serve()
			}()

			// If this test will include a call to /shutdown, listen on `shutdownChan`
			shutdownExpected := slices.Contains(tt.paths, "shutdown")
			receivedShutdown := &atomic.Bool{}
			if shutdownExpected {
				go func() {
					<-shutdownChan
					receivedShutdown.Store(true)
				}()
			}

			client := New(validAuthToken, socketPath)
			for _, path := range tt.paths {
				err := client.get(path)

				if tt.expectedError {
					require.Error(t, err)
					continue
				}

				require.NoError(t, err)
			}
			if shutdownExpected {
				require.True(t, receivedShutdown.Load())
			}
			assert.NoError(t, server.Shutdown(t.Context()))
		})
	}
}

func testSocketPath(t *testing.T) string {
	socketFileName := strings.ReplaceAll(t.Name(), "/", "_")

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

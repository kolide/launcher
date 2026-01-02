//go:build darwin || linux

package listener

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

// TestPermissions confirms that the socket file is created with appropriately-restricted permissions.
func TestPermissions(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	testPrefix := "test"
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	// Set up listener
	testListener, err := NewLauncherListener(mockKnapsack, slogger, testPrefix)
	require.NoError(t, err)
	require.NotNil(t, testListener.listener)
	t.Cleanup(func() { testListener.Interrupt(errors.New("test error")) })

	// Check permissions on socket path match what we expect -- get the security info for the socket
	fi, err := os.Stat(testListener.socketPath)
	require.NoError(t, err)
	require.Equal(t, "Srw-------", fi.Mode().String()) // 0600
}

package listener

import (
	"fmt"
	"math/rand"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewClientConn(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	prefix := "abc"

	socketPrefixWithPath := filepath.Join(rootDir, prefix)
	var mostRecentSocketPath string
	for i := range 5 {
		socketPath := fmt.Sprintf("%s_%d", socketPrefixWithPath, rand.Intn(10000))
		var lc net.ListenConfig
		listener, err := lc.Listen(t.Context(), "unix", socketPath)
		require.NoError(t, err)
		t.Cleanup(func() { listener.Close() })

		if i == 4 {
			mostRecentSocketPath = socketPath
		}
		time.Sleep(500 * time.Millisecond)
	}

	conn, err := NewLauncherClientConnection(t.Context(), rootDir, prefix)
	require.NoError(t, err)
	require.Equal(t, mostRecentSocketPath, conn.socketPath)
}

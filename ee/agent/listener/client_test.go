package listener

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolide/kit/stringutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// NewLauncherClientConnection should pick the most recent socket among many.
func TestNewClientConn(t *testing.T) {
	t.Parallel()

	var (
		rootDir              = t.TempDir()
		prefix               = "abc"
		socketPrefixWithPath = filepath.Join(rootDir, prefix)
		mostRecentSocketPath string
	)

	for i := range 5 {
		if i == 4 {
			time.Sleep(500 * time.Millisecond) // ensure clearly more recent
		}
		// MacOS path length is the limiter, preventing a UUID on the tail here.
		mostRecentSocketPath = socketPrefixWithPath + "_" + stringutil.RandomString(6)
		var lc net.ListenConfig
		listener, err := lc.Listen(t.Context(), "unix", mostRecentSocketPath)
		require.NoError(t, err)
		t.Cleanup(func() { listener.Close() })
	}

	conn, err := NewLauncherClientConnection(t.Context(), rootDir, prefix)
	require.NoError(t, err)
	require.Equal(t, mostRecentSocketPath, conn.socketPath)
}

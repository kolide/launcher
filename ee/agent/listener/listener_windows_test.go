//go:build windows

package listener

import (
	"net"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// hasPermissionsToRunTest return true if the current process has elevated permissions (administrator) --
// this is required to run tests on windows
func hasPermissionsToRunTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func dial(t *testing.T, nl net.Listener) net.Conn {
	timeout := 5 * time.Second
	conn, err := winio.DialPipe(nl.Addr().String(), &timeout)
	require.NoError(t, err)
	return conn
}

//go:build darwin || linux

package listener

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// hasPermissionsToRunTest always return true for non-windows platforms since
// elveated permissions are not required to run the tests
func hasPermissionsToRunTest() bool {
	return true
}

func dial(t *testing.T, nl net.Listener) net.Conn {
	conn, err := net.Dial("unix", nl.Addr().String())
	require.NoError(t, err)
	return conn
}

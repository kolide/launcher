package listener

import (
	"fmt"
	"net"
	"path/filepath"
)

// NewLauncherClientConnection opens up a connection to the launcher listener identified
// by the given prefix.
func NewLauncherClientConnection(rootDirectory string, socketPrefix string) (net.Conn, error) {
	socketPattern := filepath.Join(rootDirectory, socketPrefix) + "*"
	matches, err := filepath.Glob(socketPattern)
	if err != nil {
		return nil, fmt.Errorf("finding socket path at %s: %w", socketPattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no sockets found at %s", socketPattern)
	}

	// We should only ever have one match for the given directory and prefix,
	// so we return the first client connection we're able to establish.
	var clientConn net.Conn
	var lastDialErr error
	for _, match := range matches {
		clientConn, lastDialErr = net.Dial("unix", match) //nolint:noctx // will fix in https://github.com/kolide/launcher/pull/2526
		if lastDialErr != nil {
			continue
		}
		return clientConn, nil
	}

	return nil, fmt.Errorf("no connections could be opened at %+v: %w", matches, lastDialErr)
}

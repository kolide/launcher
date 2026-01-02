package listener

import (
	"fmt"
	"net"
	"path/filepath"
)

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
		clientConn, lastDialErr = net.Dial("unix", match)
		if lastDialErr != nil {
			continue
		}
		return clientConn, nil
	}

	return nil, fmt.Errorf("no connections could be opened at %+v: %w", matches, lastDialErr)
}

package listener

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
)

type clientConnection struct {
	conn net.Conn
}

type (
	genericLauncherMessage struct {
		Type string `json:"type"`
	}

	enrollmentAction struct {
		genericLauncherMessage
		EnrollmentSecret string `json:"enrollment_secret"`
	}
)

const (
	messageTypeEnroll = "enroll"
)

// NewLauncherClientConnection opens up a connection to the launcher listener identified
// by the given prefix.
func NewLauncherClientConnection(rootDirectory string, socketPrefix string) (*clientConnection, error) {
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
		return &clientConnection{
			conn: clientConn,
		}, nil
	}

	return nil, fmt.Errorf("no connections could be opened at %+v: %w", matches, lastDialErr)
}

func (c *clientConnection) Enroll(enrollSecret string) error {
	msg := enrollmentAction{
		genericLauncherMessage: genericLauncherMessage{
			Type: messageTypeEnroll,
		},
		EnrollmentSecret: enrollSecret,
	}
	rawMsg, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshalling enrollment msg: %w", err)
	}
	if _, err := c.conn.Write(rawMsg); err != nil {
		return fmt.Errorf("sending enrollment msg: %w", err)
	}
	return nil
}

func (c *clientConnection) Close() error {
	return c.conn.Close()
}

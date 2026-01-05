package listener

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"time"
)

type clientConnection struct {
	conn net.Conn
}

type (
	launcherMessage struct {
		Type    string          `json:"type"`
		MsgData json.RawMessage `json:"data"`
	}

	enrollmentAction struct {
		EnrollmentSecret string `json:"enrollment_secret"`
	}

	launcherMessageResponse struct {
		Success bool   `json:"success"`
		Message string `json:"msg"`
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
	enrollAction := enrollmentAction{
		EnrollmentSecret: enrollSecret,
	}
	rawEnrollAction, err := json.Marshal(enrollAction)
	if err != nil {
		return fmt.Errorf("marshalling enrollment action: %w", err)
	}
	msg := launcherMessage{
		Type:    messageTypeEnroll,
		MsgData: rawEnrollAction,
	}
	rawMsg, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshalling launcher msg: %w", err)
	}
	if _, err := c.conn.Write(rawMsg); err != nil {
		return fmt.Errorf("sending enrollment msg: %w", err)
	}

	// Enrollment can take over a minute and a half (there's a one-minute timeout to fetch enrollment details,
	// plus a 30-second timeout for making the enrollment request). Set a two-minute deadline for a response.
	_ = c.conn.SetDeadline(time.Now().Add(2 * time.Minute))

	// Wait for response
	var resp launcherMessageResponse
	jsonReader := json.NewDecoder(c.conn)
	if err := jsonReader.Decode(&resp); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("enrollment failed: %v", resp.Message)
	}

	return nil
}

func (c *clientConnection) Close() error {
	return c.conn.Close()
}

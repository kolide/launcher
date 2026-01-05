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
	request struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}

	enrollmentRequest struct {
		EnrollmentSecret string `json:"enrollment_secret"`
	}

	response struct {
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
	enrollReq := enrollmentRequest{
		EnrollmentSecret: enrollSecret,
	}
	rawEnrollReq, err := json.Marshal(enrollReq)
	if err != nil {
		return fmt.Errorf("marshalling enrollment request: %w", err)
	}
	req := request{
		Type: messageTypeEnroll,
		Data: rawEnrollReq,
	}
	rawReq, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshalling launcher request: %w", err)
	}
	if _, err := c.conn.Write(rawReq); err != nil {
		return fmt.Errorf("sending enrollment request: %w", err)
	}

	// Set a deadline for response
	_ = c.conn.SetDeadline(time.Now().Add(enrollTimeout))

	// Wait for response
	var resp response
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

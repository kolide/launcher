package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type clientConnection struct {
	conn       net.Conn
	socketPath string
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
	dialTimeout       = 10 * time.Second
)

// NewLauncherClientConnection opens up a connection to the launcher listener identified
// by the given prefix.
func NewLauncherClientConnection(ctx context.Context, rootDirectory string, socketPrefix string) (*clientConnection, error) {
	socketPattern := filepath.Join(rootDirectory, socketPrefix) + "*"
	matches, err := filepath.Glob(socketPattern)
	if err != nil {
		return nil, fmt.Errorf("finding socket path at %s: %w", socketPattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no sockets found at %s", socketPattern)
	}

	// We should only ever have one match for the given directory and prefix,
	// because we clean up socket files on shutdown, and also do an additional cleanup
	// check on startup, before creating a new socket file. But it is possible that
	// both of these steps could fail, resulting in multiple files -- so we check
	// the modification time for all matches, and return the most recent one.
	var mostRecentModTime time.Time
	var socketPath string
	for _, match := range matches {
		fi, err := os.Stat(match)
		if err != nil {
			continue
		}
		if fi.ModTime().After(mostRecentModTime) {
			socketPath = match
			mostRecentModTime = fi.ModTime()
		}
	}

	d := net.Dialer{
		Timeout: dialTimeout,
	}
	clientConn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", socketPath, err)
	}

	return &clientConnection{
		conn:       clientConn,
		socketPath: socketPath,
	}, nil

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

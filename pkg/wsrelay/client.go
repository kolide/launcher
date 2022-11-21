package wsrelay

import (
	"crypto/tls"
	"fmt"
	"net/url"

	"github.com/gorilla/websocket"
)

// Client is a websocket client
type Client struct {
	conn *websocket.Conn
}

// NewClient creates a new websocket client that can be interrupted
// via SIGINT
func NewClient(brokerAddr, path string, disableTLS bool, insecure bool) (*Client, error) {
	// determine the scheme
	scheme := "wss"
	if disableTLS {
		scheme = "ws"
	}

	// construct the URL to connect to
	u := url.URL{
		Scheme: scheme,
		Host:   brokerAddr,
		Path:   path,
	}

	// connect to the websocket at the given URL
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}}
	conn, resp, err := dialer.Dial(u.String(), nil)

	if err != nil {
		if err == websocket.ErrBadHandshake {
			return nil, fmt.Errorf("handshake failed with status %d: %w", resp.StatusCode, err)
		}
		return nil, err
	}

	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Read(p []byte) (n int, err error) {
	for {
		msgType, reader, err := c.conn.NextReader()
		if err != nil {
			return 0, err
		}

		if msgType != websocket.TextMessage {
			continue
		}

		return reader.Read(p)
	}
}

func (c *Client) Write(p []byte) (n int, err error) {
	writer, err := c.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return 0, err
	}
	defer writer.Close()
	return writer.Write(p)
}

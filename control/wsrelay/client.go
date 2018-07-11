package wsrelay

import (
	"crypto/tls"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

// Client is a websocket client
type Client struct {
	conn *websocket.Conn
}

// NewClient creates a new websocket client that can be interrupted
// via SIGINT
func NewClient(brokerAddr, path string, useTLS bool, insecure bool) (*Client, error) {
	// determine the scheme
	scheme := "ws"
	if useTLS {
		scheme = "wss"
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
			return nil, errors.Wrapf(err, "handshake failed with status %d", resp.StatusCode)
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

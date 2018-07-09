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
func NewClient(brokerAddr, path, secret string, useTLS bool, insecure bool) (*Client, error) {
	// determine the scheme
	var scheme string
	if useTLS {
		scheme = "wss"
	} else {
		scheme = "ws"
	}

	// construct the URL to connect to
	u := url.URL{
		Scheme: scheme,
		Host:   brokerAddr,
		Path:   path,
	}

	// set the secret in the query params
	// Note: we use this instead of an auth header because browser clients
	// can't use headers
	q := u.Query()
	q.Set("secret", secret)
	u.RawQuery = q.Encode()

	// connect to the websocket at the given URL
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}}
	conn, resp, err := dialer.Dial(u.String(), nil)

	println(u.String())
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

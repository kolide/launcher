package wsrelay

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

// Client is a websocket client
type Client struct {
	*websocket.Conn
}

// NewClient creates a new websocket client that can be interrupted
// via SIGINT
func NewClient(brokerAddr, path, secret string, useTLS bool) (*Client, error) {
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
	log.Printf("connecting to %s", u.String())

	authHeader := http.Header{"Authorization": {"Bearer " + secret}}
	// connect to the websocket at the given URL
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), authHeader)
	if err != nil {
		if err == websocket.ErrBadHandshake {
			log.Printf("handshake failed with status %d", resp.StatusCode)
		}
		if resp != nil {
			if resp.Body != nil {
				body, _ := ioutil.ReadAll(resp.Body)
				log.Printf("handshake failed with body: \n %s", string(body))
			}
		}

		return nil, err
	}

	return &Client{Conn: conn}, nil
}

func (c *Client) Read(p []byte) (n int, err error) {
	for {
		msgType, reader, err := c.NextReader()
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
	writer, err := c.NextWriter(websocket.TextMessage)
	if err != nil {
		return 0, err
	}
	defer writer.Close()
	return writer.Write(p)
}

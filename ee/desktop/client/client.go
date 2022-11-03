package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/kolide/launcher/ee/socket"
)

type transport struct {
	authToken string
	base      http.Transport
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t.authToken))
	return t.base.RoundTrip(req)
}

type client struct {
	base http.Client
}

func New(authToken, socketPath string) client {
	transport := &transport{
		authToken: authToken,
		base: http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return socket.Dial(socketPath)
			},
		},
	}

	client := client{
		base: http.Client{
			Transport: transport,
			Timeout:   1 * time.Second,
		},
	}

	return client
}

func (c *client) Shutdown() error {
	resp, err := c.base.Get("http://unix/shutdown")
	if err != nil {
		return err
	}

	if resp.Body != nil {
		resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *client) Ping() error {
	resp, err := c.base.Get("http://unix/ping")
	if err != nil {
		return err
	}

	if resp.Body != nil {
		resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

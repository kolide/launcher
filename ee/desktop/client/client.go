package client

import (
	"fmt"
	"net/http"
	"time"
)

type transport struct {
	authToken string
	base      http.Transport
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t.authToken))
	return t.base.RoundTrip(req)
}

type Client struct {
	base http.Client
}

func New(authToken, socketPath string) Client {
	transport := &transport{
		authToken: authToken,
		base: http.Transport{
			DialContext: dialContext(socketPath),
		},
	}

	client := Client{
		base: http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		},
	}

	return client
}

func (c *Client) Shutdown() error {
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

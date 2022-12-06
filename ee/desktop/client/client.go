package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

type client struct {
	baseURL *url.URL
	base    http.Client
}

// desktopUserStatus is all the device data sent to the desktop user process
type DesktopUserStatus struct {
	// TODO: Simple message format for v1, add device problem info, links to fix instructions, compliance actions...
	Status string `json:"status"`
}

func New(authToken, socketPath string) client {
	transport := &transport{
		authToken: authToken,
		base: http.Transport{
			DialContext: dialContext(socketPath),
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

func (c *client) SetStatus(st string) error {
	params := &DesktopUserStatus{
		Status: st,
	}

	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshaling json: %w", err)
	}

	resp, err := c.base.Post("http://unix/status", "application/json", bytes.NewBuffer(bodyBytes))
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

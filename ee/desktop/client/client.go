package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kolide/launcher/ee/desktop/notify"
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
	return c.get("shutdown")
}

func (c *client) Ping() error {
	return c.get("ping")
}

func (c *client) Refresh() error {
	return c.get("refresh")
}

func (c *client) Notify(title, body string) error {
	notificationToSend := notify.Notification{
		Title: title,
		Body:  body,
	}
	bodyBytes, err := json.Marshal(notificationToSend)
	if err != nil {
		return fmt.Errorf("could not marshal notification: %w", err)
	}

	resp, err := c.base.Post("http://unix/notification", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("could not send notification: %w", err)
	}

	if resp.Body != nil {
		resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *client) get(path string) error {
	resp, err := c.base.Get(fmt.Sprintf("http://unix/%s", path))
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

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kolide/launcher/ee/desktop/user/notify"
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
			Timeout:   30 * time.Second,
		},
	}

	return client
}

func (c *client) Shutdown(ctx context.Context) error {
	return c.getWithContext(ctx, "shutdown")
}

func (c *client) Ping() error {
	return c.get("ping")
}

func (c *client) Refresh() error {
	return c.get("refresh")
}

func (c *client) ShowDesktop() error {
	return c.get("show")
}

func (c *client) Notify(n notify.Notification) error {
	notificationToSend := notify.Notification{
		Title:     n.Title,
		Body:      n.Body,
		ActionUri: n.ActionUri,
	}
	bodyBytes, err := json.Marshal(notificationToSend)
	if err != nil {
		return fmt.Errorf("could not marshal notification: %w", err)
	}

	resp, err := c.base.Post("http://unix/notification", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("could not send notification: %w", err)
	}

	defer resp.Body.Close()

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

func (c *client) getWithContext(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://unix/%s", path), nil)
	if err != nil {
		return fmt.Errorf("creating request with context: %w", err)
	}

	resp, err := c.base.Do(req)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}

	if resp.Body != nil {
		resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

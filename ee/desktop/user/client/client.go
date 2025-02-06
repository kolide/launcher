package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kolide/launcher/ee/desktop/user/notify"
	"github.com/kolide/launcher/ee/desktop/user/server"
	"github.com/kolide/launcher/ee/presencedetection"
	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
			Transport: otelhttp.NewTransport(transport),
			Timeout:   30 * time.Second,
		},
	}

	return client
}

func (c *client) Shutdown(ctx context.Context) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	return c.getWithContext(ctx, "shutdown")
}

func (c *client) Ping() error {
	ctx, span := traces.StartSpan(context.TODO())
	defer span.End()

	return c.getWithContext(ctx, "ping")
}

func (c *client) Refresh() error {
	ctx, span := traces.StartSpan(context.TODO())
	defer span.End()

	return c.getWithContext(ctx, "refresh")
}

func (c *client) ShowDesktop() error {
	ctx, span := traces.StartSpan(context.TODO())
	defer span.End()

	return c.getWithContext(ctx, "show")
}

func (c *client) DetectPresence(reason string, interval time.Duration) (time.Duration, error) {
	encodedReason := url.QueryEscape(reason)
	encodedInterval := url.QueryEscape(interval.String())

	// default time out of 30s is set in New()
	timeout := c.base.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://unix/detect_presence?reason=%s&interval=%s", encodedReason, encodedInterval), nil)
	if err != nil {
		return presencedetection.DetectionFailedDurationValue, fmt.Errorf("creating presence request: %w", err)
	}
	resp, requestErr := c.base.Do(req)
	if requestErr != nil {
		return presencedetection.DetectionFailedDurationValue, fmt.Errorf("getting presence: %w", requestErr)
	}

	var response server.DetectPresenceResponse
	if resp.Body != nil {
		defer resp.Body.Close()

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return presencedetection.DetectionFailedDurationValue, fmt.Errorf("decoding response: %w", err)
		}
	}

	var detectionErr error
	if response.Error != "" {
		detectionErr = errors.New(response.Error)
	}

	durationSinceLastDetection, parseErr := time.ParseDuration(response.DurationSinceLastDetection)
	if parseErr != nil {
		return presencedetection.DetectionFailedDurationValue, fmt.Errorf("parsing time since last detection: %w", parseErr)
	}

	return durationSinceLastDetection, detectionErr
}

func (c *client) CreateSecureEnclaveKey() ([]byte, error) {
	timeout := c.base.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/secure_enclave_key", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating secure enclave key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.base.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting secure enclave key: %w", err)
	}

	if resp.Body == nil {
		return nil, errors.New("response body is nil")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}

func (c *client) Notify(n notify.Notification) error {
	timeout := c.base.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	notificationToSend := notify.Notification{
		Title:     n.Title,
		Body:      n.Body,
		ActionUri: n.ActionUri,
	}
	bodyBytes, err := json.Marshal(notificationToSend)
	if err != nil {
		return fmt.Errorf("could not marshal notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/notification", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.base.Do(req)
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
	return c.getWithContext(context.Background(), path)
}

func (c *client) getWithContext(ctx context.Context, path string) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	timeout := c.base.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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

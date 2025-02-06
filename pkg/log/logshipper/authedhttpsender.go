package logshipper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type authedHttpSender struct {
	endpoint  string
	authtoken string
	client    *http.Client
}

func newAuthHttpSender() *authedHttpSender {
	return &authedHttpSender{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (a *authedHttpSender) Send(r io.Reader) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("authorization", a.authtoken)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyData, err := io.ReadAll(resp.Request.Body)
		if err != nil {
			return fmt.Errorf("received non 200 http status code: %d, error reading body response body %w", resp.StatusCode, err)
		}

		escapedBodyData := strings.ReplaceAll(strings.ReplaceAll(string(bodyData), "\n", ""), "\r", "") // remove any newlines
		return fmt.Errorf("received non 200 http status code: %d, response body: %s", resp.StatusCode, escapedBodyData)
	}
	return nil
}

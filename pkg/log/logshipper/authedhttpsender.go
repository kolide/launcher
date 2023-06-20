package logshipper

import (
	"fmt"
	"io"
	"net/http"
)

type authedHttpSender struct {
	endpoint  string
	authtoken string
}

func newAuthHttpSender(endpoint, authtoken string) *authedHttpSender {
	return &authedHttpSender{
		endpoint:  endpoint,
		authtoken: authtoken,
	}
}

func (a *authedHttpSender) Send(r io.Reader) error {
	req, err := http.NewRequest("POST", a.endpoint, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.authtoken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyData, err := io.ReadAll(resp.Request.Body)
		if err != nil {
			return fmt.Errorf("received non 200 http status code: %d, error reading body response body %w", resp.StatusCode, err)
		}

		return fmt.Errorf("received non 200 http status code: %d, response body: %s", resp.StatusCode, bodyData)
	}
	return nil
}

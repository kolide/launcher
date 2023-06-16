package sendbuffer

import (
	"fmt"
	"io"
	"net/http"
)

type httpsender struct {
	endpoint string
}

func NewHttpSender(endpoint string) *httpsender {
	return &httpsender{
		endpoint: endpoint,
	}
}

func (s *httpsender) Send(r io.Reader) error {
	resp, err := http.Post(s.endpoint, "application/octet-stream", r)
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

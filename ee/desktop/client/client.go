package client

import (
	"fmt"
	"net/http"
)

func Shutdown(authToken, socketPath string) error {
	client := http.Client{
		Transport: &http.Transport{
			DialContext: dialContext(socketPath),
		},
	}

	request, err := http.NewRequest("GET", "http://unix/shutdown", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

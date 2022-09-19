package client

import "net/http"

func Shutdown(socketPath string) error {
	client := http.Client{
		Transport: &http.Transport{
			DialContext: dialContext(socketPath),
		},
	}

	resp, err := client.Get("http://unix/shutdown")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

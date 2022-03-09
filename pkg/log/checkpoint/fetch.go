package checkpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type httpClient interface {
	Get(url string) (resp *http.Response, err error)
}

func fetchFromUrls(client httpClient, urls ...string) map[string]interface{} {

	results := make(map[string]interface{})

	for _, url := range urls {
		response, err := fetchFromUrl(client, url)
		if err != nil {
			results[url] = err.Error()
		} else {
			results[url] = response
		}
	}

	return results
}

type notaryRelease struct {
	Signed struct {
		Version int `json:"version"`
	} `json:"signed"`
}

func fetchFromUrl(client httpClient, url string) (string, error) {
	response, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("[%s] %s", response.Status, string(bytes)), nil
}

func fetchNotaryVersion(client httpClient, url string) map[string]interface{} {
	results := make(map[string]interface{})
	response, err := client.Get(url)
	if err != nil {
		results[url] = err.Error()
		return results
	}
	defer response.Body.Close()

	var notaryRelease notaryRelease
	if err := json.NewDecoder(response.Body).Decode(&notaryRelease); err != nil {
		results[url] = err.Error()
		return results
	}

	results[url] = notaryRelease.Signed.Version
	return results
}

package checkpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type httpClient interface {
	Get(url string) (resp *http.Response, err error)
}

func fetchFromUrls(client httpClient, urls ...*url.URL) map[string]string {

	results := make(map[string]string)

	for _, url := range urls {
		response, err := fetchFromUrl(client, url)
		if err != nil {
			results[url.String()] = err.Error()
		} else {
			results[url.String()] = response
		}
	}

	return results
}

type notaryRelease struct {
	Signed struct {
		Version int `json:"version"`
	} `json:"signed"`
}

func fetchFromUrl(client httpClient, url *url.URL) (string, error) {
	response, err := client.Get(url.String())
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s", response.Status, string(bytes)), nil
}

func fetchNotaryVersions(client httpClient, urls ...*url.URL) map[string]string {
	results := make(map[string]string)

	for _, url := range urls {
		response, err := fetchNotaryVersion(client, url)
		if err != nil {
			results[url.String()] = err.Error()
		} else {
			results[url.String()] = response
		}
	}

	return results
}

func fetchNotaryVersion(client httpClient, url *url.URL) (string, error) {
	response, err := client.Get(url.String())
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	var notaryRelease notaryRelease
	if err := json.NewDecoder(response.Body).Decode(&notaryRelease); err != nil {
		return "", err
	}

	return strconv.Itoa(notaryRelease.Signed.Version), nil
}

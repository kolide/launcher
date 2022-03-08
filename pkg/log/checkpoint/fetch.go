package checkpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type httpClient interface {
	Get(url string) (resp *http.Response, err error)
}

func fetchFromUrls(client httpClient, urls []string) []string {

	results := []string{}

	for _, url := range urls {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s: ", url))

		response, err := fetchFromUrl(client, url)
		if err != nil {
			sb.WriteString(err.Error())
		} else {
			sb.WriteString(response)
		}

		results = append(results, sb.String())
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

func fetchNotaryVersion(client httpClient, url string) string {
	const strFmt = "%s: %s"
	response, err := client.Get(url)
	if err != nil {
		return fmt.Sprintf(strFmt, url, err.Error())
	}
	defer response.Body.Close()

	var notaryRelease notaryRelease
	if err := json.NewDecoder(response.Body).Decode(&notaryRelease); err != nil {
		return fmt.Sprintf(strFmt, url, err.Error())
	}

	return fmt.Sprintf(strFmt, url, strconv.Itoa(notaryRelease.Signed.Version))
}

package checkups

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/kolide/launcher/pkg/agent/types"
)

type (
	serverVersionCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]string
	}
)

func (svc *serverVersionCheckup) Data() any             { return svc.data }
func (svc *serverVersionCheckup) ExtraFileName() string { return "" }
func (svc *serverVersionCheckup) Name() string          { return "Server Version" }
func (svc *serverVersionCheckup) Status() Status        { return svc.status }
func (svc *serverVersionCheckup) Summary() string       { return svc.summary }

func (svc *serverVersionCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	svc.data = make(map[string]string)
	if !svc.k.KolideHosted() {
		return nil
	}

	httpClient := &http.Client{Timeout: requestTimeout}
	serverEndpoint := fmt.Sprintf("%s/version", svc.k.KolideServerURL())
	serverUrl, err := parseUrl(svc.k, serverEndpoint)
	if err != nil {
		return err
	}

	response, err := fetchFromUrl(httpClient, serverUrl)
	if err != nil {
		svc.data[serverUrl.String()] = err.Error()
		svc.status = Failing
		svc.summary = "Unable to gather server version response"
	} else {
		svc.data[serverUrl.String()] = response
		svc.status = Passing
		svc.summary = fmt.Sprintf("%s returned version response: %s", serverUrl.String(), response)
	}

	return nil
}

func fetchFromUrl(client *http.Client, url *url.URL) (string, error) {
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

package checkups

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kolide/launcher/pkg/agent/types"
)

type (
	notaryCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}

	notaryRelease struct {
		Signed struct {
			Version int `json:"version"`
		} `json:"signed"`
	}
)

func (nc *notaryCheckup) Data() any             { return nc.data }
func (nc *notaryCheckup) ExtraFileName() string { return "" }
func (nc *notaryCheckup) Name() string          { return "Notary Version" }
func (nc *notaryCheckup) Status() Status        { return nc.status }
func (nc *notaryCheckup) Summary() string       { return nc.summary }

func (nc *notaryCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	nc.data = make(map[string]any)
	if !nc.k.KolideHosted() || !nc.k.Autoupdate() {
		nc.status = Unknown
		if !nc.k.KolideHosted() {
			nc.summary = "not kolide hosted"
		} else {
			nc.summary = "autoupdates are not enabled"
		}

		return nil
	}

	httpClient := &http.Client{Timeout: requestTimeout}
	notaryEndpoint := fmt.Sprintf("%s/v2/kolide/launcher/_trust/tuf/targets/releases.json", nc.k.NotaryServerURL())
	notaryUrl, err := parseUrl(nc.k, notaryEndpoint)
	if err != nil {
		return err
	}

	response, err := fetchNotaryVersion(httpClient, notaryUrl)
	if err != nil {
		nc.data[notaryUrl.String()] = err.Error()
		nc.status = Failing
		nc.summary = fmt.Sprintf("Unable to gather notary version response from %s", notaryUrl.String())
		return nil
	}

	nc.data[notaryUrl.String()] = response
	nc.status = Passing
	nc.summary = fmt.Sprintf("Successfully gathered notary version %s from %s", response, notaryUrl.String())
	return nil
}

func fetchNotaryVersion(client *http.Client, url *url.URL) (string, error) {
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

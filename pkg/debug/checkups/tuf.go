package checkups

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
)

type (
	tufCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}

	customTufRelease struct {
		Custom struct {
			Target string `json:"target"`
		} `json:"custom"`
	}

	tufRelease struct {
		Signed struct {
			Targets map[string]customTufRelease `json:"targets"`
		} `json:"signed"`
	}
)

func (tc *tufCheckup) Data() map[string]any  { return tc.data }
func (tc *tufCheckup) ExtraFileName() string { return "" }
func (tc *tufCheckup) Name() string          { return "Tuf Version" }
func (tc *tufCheckup) Status() Status        { return tc.status }
func (tc *tufCheckup) Summary() string       { return tc.summary }

func (tc *tufCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	tc.data = make(map[string]any)
	if !tc.k.Autoupdate() {
		return nil
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	tufEndpoint := fmt.Sprintf("%s/repository/targets.json", tc.k.TufServerURL())
	tufUrl, err := parseUrl(tc.k, tufEndpoint)
	if err != nil {
		return err
	}

	response, err := tc.fetchTufVersion(httpClient, tufUrl)
	if err != nil {
		tc.status = Erroring
		tc.data[tufUrl.String()] = err.Error()
		tc.summary = "Unable to gather tuf version response"
		return nil
	}

	if response == "" {
		tc.status = Failing
		tc.data[tufUrl.String()] = "missing from tuf response"
		tc.summary = "missing version from tuf targets response"
		return nil
	}

	tc.status = Passing
	tc.data[tufUrl.String()] = response
	tc.summary = fmt.Sprintf("Successfully gathered release version %s from %s", response, tufUrl.String())

	return nil
}

// fetchTufVersion retrieves the latest targets.json from the tuf URL provided.
// We're attempting to key into the current target for the current platform here,
// which should look like:
// https://[TUF_HOST]/repository/targets.json -> full targets blob
// ---> signed -> targets -> launcher/<GOOS>/<GOARCH|universal>/<RELEASE_CHANNEL>/release.json -> custom -> target
func (tc *tufCheckup) fetchTufVersion(client *http.Client, url *url.URL) (string, error) {
	response, err := client.Get(url.String())
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	var releaseTargets tufRelease
	if err := json.NewDecoder(response.Body).Decode(&releaseTargets); err != nil {
		return "", err
	}

	upgradePathKey := fmt.Sprintf("launcher/%s/%s/%s/release.json", runtime.GOOS, tuf.PlatformArch(), tc.k.UpdateChannel())
	hostTargets, ok := releaseTargets.Signed.Targets[upgradePathKey]

	if !ok {
		return "", fmt.Errorf("unable to find matching release data for %s", upgradePathKey)
	}

	return hostTargets.Custom.Target, nil
}

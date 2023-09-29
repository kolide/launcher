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
	tuffCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]string
	}

	customTuffRelease struct {
		Custom struct {
			Target string `json:"target"`
		} `json:"custom"`
	}

	tuffRelease struct {
		Signed struct {
			Targets map[string]customTuffRelease `json:"targets"`
		} `json:"signed"`
	}
)

func (tc *tuffCheckup) Data() any             { return tc.data }
func (tc *tuffCheckup) ExtraFileName() string { return "" }
func (tc *tuffCheckup) Name() string          { return "Tuff Version" }
func (tc *tuffCheckup) Status() Status        { return tc.status }
func (tc *tuffCheckup) Summary() string       { return tc.summary }

func (tc *tuffCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	tc.data = make(map[string]string)
	if !tc.k.Autoupdate() {
		return nil
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	tuffEndpoint := fmt.Sprintf("%s/repository/targets.json", tc.k.TufServerURL())
	tuffUrl, err := parseUrl(tc.k, tuffEndpoint)
	if err != nil {
		return err
	}

	response, err := tc.fetchTuffVersion(httpClient, tuffUrl)
	if err != nil {
		tc.status = Erroring
		tc.data[tuffUrl.String()] = err.Error()
		tc.summary = "Unable to gather tuff version response"
		return nil
	}

	if response == "" {
		tc.status = Failing
		tc.data[tuffUrl.String()] = "missing from tuff response"
		tc.summary = "missing version from tuff targets response"
		return nil
	}

	tc.status = Passing
	tc.data[tuffUrl.String()] = response
	tc.summary = fmt.Sprintf("Successfully gathered release version %s from %s", response, tuffUrl.String())

	return nil
}

// fetchTuffVersion retrieves the latest targets.json from the tuff URL provided.
// We're attempting to key into the current target for the current platform here,
// which should look like:
// https://[TUFF_HOST]/repository/targets.json -> full targets blob
// ---> signed -> targets -> launcher/<GOOS>/<GOARCH|universal>/<RELEASE_CHANNEL>/release.json -> custom -> target
func (tc *tuffCheckup) fetchTuffVersion(client *http.Client, url *url.URL) (string, error) {
	response, err := client.Get(url.String())
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	var releaseTargets tuffRelease
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

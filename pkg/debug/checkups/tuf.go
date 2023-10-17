package checkups

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-kit/kit/log"
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
			Version int                         `json:"version"`
		} `json:"signed"`
	}
)

func (tc *tufCheckup) Data() map[string]any  { return tc.data }
func (tc *tufCheckup) ExtraFileName() string { return "tuf.json" }
func (tc *tufCheckup) Name() string          { return "TUF" }
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

	// Primarily, we want to validate that we can talk to the TUF server
	releaseVersion, remoteMetadataVersion, err := tc.remoteTufMetadata(httpClient, tufUrl)
	if err != nil {
		tc.status = Erroring
		tc.data[tufUrl.String()] = err.Error()
		tc.summary = "Unable to gather tuf version response"
		return nil
	}

	if releaseVersion == "" {
		tc.status = Failing
		tc.data[tufUrl.String()] = "missing from tuf response"
		tc.summary = "missing version from tuf targets response"
		return nil
	}

	tc.status = Passing
	tc.data[tufUrl.String()] = releaseVersion
	tc.summary = fmt.Sprintf("Successfully gathered release version %s from %s", releaseVersion, tufUrl.String())

	// Gather additional data only if we're running flare
	if extraFH == io.Discard {
		return nil
	}

	tufData := map[string]any{
		"remote_metadata_version": remoteMetadataVersion,
	}

	if localMetadataVersion, err := tc.localTufMetadata(); err != nil {
		tufData["local_metadata_version"] = fmt.Sprintf("error getting local metadata version: %v", err)
	} else {
		tufData["local_metadata_version"] = localMetadataVersion
	}

	if localLauncherVersions, err := tc.versionsInLauncherLibrary(); err != nil {
		tufData["launcher_versions_in_library"] = fmt.Sprintf("error getting versions available in local launcher library: %v", err)
	} else {
		tufData["launcher_versions_in_library"] = localLauncherVersions
	}

	if localOsqueryVersions, err := tc.versionsInOsquerydLibrary(); err != nil {
		tufData["osqueryd_versions_in_library"] = fmt.Sprintf("error getting versions available in local osqueryd library: %v", err)
	} else {
		tufData["osqueryd_versions_in_library"] = localOsqueryVersions
	}

	tufData["selected_versions"] = tc.selectedVersions()

	if b, err := json.Marshal(tufData); err == nil {
		_, _ = extraFH.Write(b)
	}

	return nil
}

// remoteTufMetadata retrieves the latest targets.json from the tuf URL provided.
// We're attempting to key into the current target for the current platform here,
// which should look like:
// https://[TUF_HOST]/repository/targets.json -> full targets blob
// ---> signed -> targets -> launcher/<GOOS>/<GOARCH|universal>/<RELEASE_CHANNEL>/release.json -> custom -> target
func (tc *tufCheckup) remoteTufMetadata(client *http.Client, tufUrl *url.URL) (string, int, error) {
	response, err := client.Get(tufUrl.String())
	if err != nil {
		return "", 0, err
	}
	defer response.Body.Close()

	var releaseTargets tufRelease
	if err := json.NewDecoder(response.Body).Decode(&releaseTargets); err != nil {
		return "", 0, err
	}

	upgradePathKey := fmt.Sprintf("launcher/%s/%s/%s/release.json", runtime.GOOS, tuf.PlatformArch(), tc.k.UpdateChannel())
	hostTargets, ok := releaseTargets.Signed.Targets[upgradePathKey]

	if !ok {
		return "", releaseTargets.Signed.Version, fmt.Errorf("unable to find matching release data for %s", upgradePathKey)
	}

	return hostTargets.Custom.Target, releaseTargets.Signed.Version, nil
}

// localTufMetadata inspects the local target metadata to extract the metadata version.
// We can compare this version to the one retrieved by `remoteTufMetadata` to see whether
// this installation of launcher is successfully updating its TUF repository.
func (tc *tufCheckup) localTufMetadata() (int, error) {
	targetsMetadataFile := filepath.Join(tuf.LocalTufDirectory(tc.k.RootDirectory()), "targets.json")
	b, err := os.ReadFile(targetsMetadataFile)
	if err != nil {
		return 0, fmt.Errorf("reading file %s: %w", targetsMetadataFile, err)
	}

	var releaseTargets tufRelease
	if err := json.Unmarshal(b, &releaseTargets); err != nil {
		return 0, fmt.Errorf("unmarshalling file %s: %w", targetsMetadataFile, err)
	}

	return releaseTargets.Signed.Version, nil
}

// versionsInLauncherLibrary returns all updates available in the launcher update directory.
func (tc *tufCheckup) versionsInLauncherLibrary() (string, error) {
	updatesDir := tc.k.UpdateDirectory()
	if updatesDir == "" {
		updatesDir = tuf.DefaultLibraryDirectory(tc.k.RootDirectory())
	}

	launcherVersionMatchPattern := filepath.Join(updatesDir, "launcher", "*")
	launcherMatches, err := filepath.Glob(launcherVersionMatchPattern)
	if err != nil {
		return "", fmt.Errorf("globbing for launcher matches at %s: %w", launcherVersionMatchPattern, err)
	}

	launcherVersions := make([]string, len(launcherMatches))
	for i := 0; i < len(launcherMatches); i += 1 {
		launcherVersions[i] = filepath.Base(launcherMatches[i])
	}

	return strings.Join(launcherVersions, ","), nil
}

// versionsInOsquerydLibrary returns all updates available in the osqueryd update directory.
func (tc *tufCheckup) versionsInOsquerydLibrary() (string, error) {
	updatesDir := tc.k.UpdateDirectory()
	if updatesDir == "" {
		updatesDir = tuf.DefaultLibraryDirectory(tc.k.RootDirectory())
	}

	osquerydVersionMatchPattern := filepath.Join(updatesDir, "osqueryd", "*")
	osquerydMatches, err := filepath.Glob(osquerydVersionMatchPattern)
	if err != nil {
		return "", fmt.Errorf("globbing for osqueryd matches at %s: %w", osquerydVersionMatchPattern, err)
	}

	osquerydVersions := make([]string, len(osquerydMatches))
	for i := 0; i < len(osquerydMatches); i += 1 {
		osquerydVersions[i] = filepath.Base(osquerydMatches[i])
	}

	return strings.Join(osquerydVersions, ","), nil
}

// selectedVersions returns the versions of launcher and osqueryd that the current
// installation would select as the correct version to run.
func (tc *tufCheckup) selectedVersions() map[string]map[string]string {
	selectedVersions := map[string]map[string]string{
		"launcher": make(map[string]string),
		"osqueryd": make(map[string]string),
	}

	if launcherVersion, err := tuf.CheckOutLatestWithoutConfig("launcher", log.NewNopLogger()); err != nil {
		selectedVersions["launcher"]["path"] = fmt.Sprintf("error checking out latest version: %v", err)
	} else {
		selectedVersions["launcher"]["path"] = launcherVersion.Path
		selectedVersions["launcher"]["version"] = launcherVersion.Version
	}

	if osquerydVersion, err := tuf.CheckOutLatestWithoutConfig("osqueryd", log.NewNopLogger()); err != nil {
		selectedVersions["osqueryd"]["path"] = fmt.Sprintf("error checking out latest version: %v", err)
	} else {
		selectedVersions["osqueryd"]["path"] = osquerydVersion.Path
		selectedVersions["osqueryd"]["version"] = osquerydVersion.Version
	}

	return selectedVersions
}

// Package tuf provides an autoupdater that uses our new TUF infrastructure,
// replacing the previous Notary-based implementation. It allows launcher to
// download new launcher and osqueryd binaries.
package tuf

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	client "github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
	"github.com/theupdateframework/go-tuf/data"
)

//go:embed assets/tuf/root.json
var rootJson []byte

// Configuration defaults
const (
	tufDirectoryName = "tuf"
)

// Binaries handled by autoupdater
type autoupdatableBinary string

const (
	binaryLauncher autoupdatableBinary = "launcher"
	binaryOsqueryd autoupdatableBinary = "osqueryd"
)

var binaries = []autoupdatableBinary{binaryLauncher, binaryOsqueryd}

var autoupdatableBinaryMap = map[string]autoupdatableBinary{
	"launcher": binaryLauncher,
	"osqueryd": binaryOsqueryd,
}

type ReleaseFileCustomMetadata struct {
	Target string `json:"target"`
}

// Control server subsystem (used to send "update now" commands)
const AutoupdateSubsystemName = "autoupdate"

type (
	controlServerAutoupdateRequest struct {
		BinariesToUpdate   []binaryToUpdate `json:"binaries_to_update"`
		BypassInitialDelay bool             `json:"bypass_initial_delay,omitempty"`
	}

	// In the future, we may allow for setting a particular version here as well
	binaryToUpdate struct {
		Name string `json:"name"`
	}
)

type librarian interface {
	Available(binary autoupdatableBinary, targetFilename string) bool
	AddToLibrary(binary autoupdatableBinary, currentVersion string, targetFilename string, targetMetadata data.TargetFileMeta) error
	TidyLibrary(binary autoupdatableBinary, currentVersion string)
}

type querier interface {
	Query(query string) ([]map[string]string, error)
}

type TufAutoupdater struct {
	metadataClient         *client.Client
	libraryManager         librarian
	osquerier              querier // used to query for current running osquery version
	osquerierRetryInterval time.Duration
	knapsack               types.Knapsack
	store                  types.KVStore // stores autoupdater errors for kolide_tuf_autoupdater_errors table
	updateChannel          string
	pinnedVersions         map[autoupdatableBinary]string        // maps the binaries to their pinned versions
	pinnedVersionGetters   map[autoupdatableBinary]func() string // maps the binaries to the knapsack function to retrieve updated pinned versions
	initialDelayEnd        time.Time
	updateLock             *sync.Mutex
	interrupt              chan struct{}
	interrupted            atomic.Bool
	signalRestart          chan error
	slogger                *slog.Logger
	restartFuncs           map[autoupdatableBinary]func(context.Context) error
}

type TufAutoupdaterOption func(*TufAutoupdater)

func WithOsqueryRestart(restart func(context.Context) error) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		if ta.restartFuncs == nil {
			ta.restartFuncs = make(map[autoupdatableBinary]func(context.Context) error)
		}
		ta.restartFuncs[binaryOsqueryd] = restart
	}
}

func NewTufAutoupdater(ctx context.Context, k types.Knapsack, metadataHttpClient *http.Client, mirrorHttpClient *http.Client,
	osquerier querier, opts ...TufAutoupdaterOption) (*TufAutoupdater, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	ta := &TufAutoupdater{
		knapsack:      k,
		interrupt:     make(chan struct{}, 1),
		signalRestart: make(chan error, 1),
		store:         k.AutoupdateErrorsStore(),
		updateChannel: k.UpdateChannel(),
		pinnedVersions: map[autoupdatableBinary]string{
			binaryLauncher: k.PinnedLauncherVersion(), // empty string if not pinned
			binaryOsqueryd: k.PinnedOsquerydVersion(), // ditto
		},
		pinnedVersionGetters: map[autoupdatableBinary]func() string{
			binaryLauncher: func() string { return k.PinnedLauncherVersion() },
			binaryOsqueryd: func() string { return k.PinnedOsquerydVersion() },
		},
		initialDelayEnd:        time.Now().Add(k.AutoupdateInitialDelay()),
		updateLock:             &sync.Mutex{},
		osquerier:              osquerier,
		osquerierRetryInterval: 30 * time.Second,
		slogger:                k.Slogger().With("component", "tuf_autoupdater"),
		restartFuncs:           make(map[autoupdatableBinary]func(context.Context) error),
	}

	for _, opt := range opts {
		opt(ta)
	}

	var err error
	ta.metadataClient, err = initMetadataClient(ctx, k.RootDirectory(), k.TufServerURL(), metadataHttpClient)
	if err != nil {
		return nil, fmt.Errorf("could not init metadata client: %w", err)
	}

	// If the update directory wasn't set by a flag, use the default location of <launcher root>/updates.
	updateDirectory := k.UpdateDirectory()
	if updateDirectory == "" {
		updateDirectory = DefaultLibraryDirectory(k.RootDirectory())
	}
	ta.libraryManager, err = newUpdateLibraryManager(k.MirrorServerURL(), mirrorHttpClient, updateDirectory, k.Slogger())
	if err != nil {
		return nil, fmt.Errorf("could not init update library manager: %w", err)
	}

	// Subscribe to changes in update-related flags
	ta.knapsack.RegisterChangeObserver(ta, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion)

	return ta, nil
}

// initMetadataClient sets up a TUF client with our validated root metadata, prepared to fetch updates
// from our TUF server.
func initMetadataClient(ctx context.Context, rootDirectory, metadataUrl string, metadataHttpClient *http.Client) (*client.Client, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	// Set up the local TUF directory for our TUF client
	localTufDirectory := LocalTufDirectory(rootDirectory)
	if err := os.MkdirAll(localTufDirectory, 0750); err != nil {
		return nil, fmt.Errorf("could not make local TUF directory %s: %w", localTufDirectory, err)
	}

	// Ensure that directory permissions are correct, otherwise TUF will fail to initialize. We cannot
	// have permissions in excess of -rwxr-x---.
	if err := os.Chmod(localTufDirectory, 0750); err != nil {
		return nil, fmt.Errorf("chmodding local TUF directory %s: %w", localTufDirectory, err)
	}

	// Set up our local store i.e. point to the directory in our filesystem
	localStore, err := filejsonstore.NewFileJSONStore(localTufDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not initialize local TUF store: %w", err)
	}

	// Set up our remote store i.e. tuf.kolide.com
	remoteOpts := client.HTTPRemoteOptions{
		MetadataPath: "/repository",
	}
	remoteStore, err := client.HTTPRemoteStore(metadataUrl, &remoteOpts, metadataHttpClient)
	if err != nil {
		return nil, fmt.Errorf("could not initialize remote TUF store: %w", err)
	}

	metadataClient := client.NewClient(localStore, remoteStore)
	if err := metadataClient.Init(rootJson); err != nil {
		return nil, fmt.Errorf("failed to initialize TUF client with root JSON: %w", err)
	}

	return metadataClient, nil
}

func LocalTufDirectory(rootDirectory string) string {
	return filepath.Join(rootDirectory, tufDirectoryName)
}

func DefaultLibraryDirectory(rootDirectory string) string {
	return filepath.Join(rootDirectory, "updates")
}

// Execute is the TufAutoupdater run loop. It periodically checks to see if a new release
// has been published; less frequently, it removes old/outdated TUF errors from the bucket
// we store them in.
func (ta *TufAutoupdater) Execute() (err error) {
	// Delay startup, if initial delay is set
	select {
	case <-ta.interrupt:
		ta.slogger.Log(context.TODO(), slog.LevelDebug,
			"received external interrupt during initial delay, stopping",
		)
		return nil
	case signalRestartErr := <-ta.signalRestart:
		ta.slogger.Log(context.TODO(), slog.LevelDebug,
			"received interrupt to restart launcher after update during initial delay, stopping",
		)
		return signalRestartErr
	case <-time.After(ta.knapsack.AutoupdateInitialDelay()):
		break
	}

	ta.slogger.Log(context.TODO(), slog.LevelInfo,
		"exiting initial delay",
	)

	// For now, tidy the library on startup. In the future, we will tidy the library
	// earlier, after version selection.
	ta.tidyLibrary()

	checkTicker := time.NewTicker(ta.knapsack.AutoupdateInterval())
	defer checkTicker.Stop()
	cleanupTicker := time.NewTicker(12 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		ta.slogger.Log(context.TODO(), slog.LevelInfo,
			"checking for updates",
		)
		if err := ta.checkForUpdate(context.TODO(), binaries); err != nil {
			ta.storeError(err)
			ta.slogger.Log(context.TODO(), slog.LevelError,
				"error checking for update",
				"err", err,
			)
		} else {
			ta.slogger.Log(context.TODO(), slog.LevelInfo,
				"completed check for update",
			)
		}

		select {
		case <-checkTicker.C:
			continue
		case <-cleanupTicker.C:
			ta.cleanUpOldErrors()
		case <-ta.interrupt:
			ta.slogger.Log(context.TODO(), slog.LevelDebug,
				"received external interrupt, stopping",
			)
			return nil
		case signalRestartErr := <-ta.signalRestart:
			ta.slogger.Log(context.TODO(), slog.LevelDebug,
				"received interrupt to restart launcher after update, stopping",
			)
			return signalRestartErr
		}
	}
}

func (ta *TufAutoupdater) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if ta.interrupted.Load() {
		return
	}
	ta.interrupted.Store(true)

	ta.interrupt <- struct{}{}
}

// Do satisfies the actionqueue.actor interface; it allows the control server to send
// requests down to autoupdate immediately.
func (ta *TufAutoupdater) Do(data io.Reader) error {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	var updateRequest controlServerAutoupdateRequest
	if err := json.NewDecoder(data).Decode(&updateRequest); err != nil {
		ta.slogger.Log(ctx, slog.LevelWarn,
			"received update request in unexpected format from control server, discarding",
			"err", err,
		)
		// We don't return an error because we don't want the actionqueue to retry this request
		return nil
	}

	if time.Now().Before(ta.initialDelayEnd) && !updateRequest.BypassInitialDelay {
		ta.slogger.Log(ctx, slog.LevelWarn,
			"received update request during initial delay, discarding",
			"initial_delay_end", ta.initialDelayEnd.UTC().Format(time.RFC3339),
		)
		// We don't return an error because there's no need for the actionqueue to retry this request --
		// we're going to perform an autoupdate check as soon as we exit the delay anyway.
		return nil
	}

	binariesToUpdate := make([]autoupdatableBinary, 0)
	for _, b := range updateRequest.BinariesToUpdate {
		if val, ok := autoupdatableBinaryMap[b.Name]; ok {
			binariesToUpdate = append(binariesToUpdate, val)
			continue
		}
		ta.slogger.Log(ctx, slog.LevelWarn,
			"received request from control server autoupdate unknown binary, ignoring",
			"unknown_binary", b.Name,
		)
	}

	if len(binariesToUpdate) == 0 {
		ta.slogger.Log(ctx, slog.LevelDebug,
			"received request from control server to check for update now, but no valid binaries specified in request",
		)
		return nil
	}

	ta.slogger.Log(ctx, slog.LevelInfo,
		"received request from control server to check for update now",
		"binaries_to_update", fmt.Sprintf("%+v", binariesToUpdate),
	)

	if err := ta.checkForUpdate(ctx, binariesToUpdate); err != nil {
		ta.storeError(err)
		ta.slogger.Log(ctx, slog.LevelError,
			"error checking for update per control server request",
			"binaries_to_update", fmt.Sprintf("%+v", binariesToUpdate),
			"err", err,
		)

		return fmt.Errorf("could not check for update: %w", err)
	}

	ta.slogger.Log(ctx, slog.LevelInfo,
		"successfully checked for update per control server request",
		"binaries_to_update", fmt.Sprintf("%+v", binariesToUpdate),
	)

	return nil
}

// FlagsChanged satisfies the FlagsChangeObserver interface, allowing the autoupdater
// to respond to changes to autoupdate-related settings.
func (ta *TufAutoupdater) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	binariesToCheckForUpdate := make([]autoupdatableBinary, 0)

	// Check to see if update channel has changed
	if ta.updateChannel != ta.knapsack.UpdateChannel() {
		ta.slogger.Log(ctx, slog.LevelInfo,
			"control server sent down new update channel value",
			"new_channel", ta.knapsack.UpdateChannel(),
			"old_channel", ta.updateChannel,
		)
		ta.updateChannel = ta.knapsack.UpdateChannel()
		binariesToCheckForUpdate = append(binariesToCheckForUpdate, binaryLauncher, binaryOsqueryd)
	}

	// Check to see if pinned versions have changed
	for binary, currentPinnedVersion := range ta.pinnedVersions {
		newPinnedVersion := ta.pinnedVersionGetters[binary]()
		if currentPinnedVersion != newPinnedVersion {
			ta.slogger.Log(ctx, slog.LevelInfo,
				"control server sent down new pinned version for binary",
				"binary", binary,
				"new_pinned_version", newPinnedVersion,
				"old_pinned_version", currentPinnedVersion,
			)
			ta.pinnedVersions[binary] = newPinnedVersion
			if !slices.Contains(binariesToCheckForUpdate, binary) {
				binariesToCheckForUpdate = append(binariesToCheckForUpdate, binary)
			}
		}
	}

	// No updates, or we're in the initial delay
	if len(binariesToCheckForUpdate) == 0 || time.Now().Before(ta.initialDelayEnd) {
		return
	}

	// At least one binary requires a recheck -- perform that now
	if err := ta.checkForUpdate(ctx, binariesToCheckForUpdate); err != nil {
		ta.storeError(err)
		ta.slogger.Log(ctx, slog.LevelError,
			"error checking for update after autoupdate setting changed",
			"update_channel", ta.updateChannel,
			"pinned_launcher_version", ta.knapsack.PinnedLauncherVersion(),
			"pinned_osqueryd_version", ta.knapsack.PinnedOsquerydVersion(),
			"err", err,
		)
		return
	}

	ta.slogger.Log(ctx, slog.LevelInfo,
		"checked for update after autoupdate setting changed",
		"update_channel", ta.updateChannel,
		"pinned_launcher_version", ta.knapsack.PinnedLauncherVersion(),
		"pinned_osqueryd_version", ta.knapsack.PinnedOsquerydVersion(),
	)
}

// tidyLibrary gets the current running version for each binary (so that the current version is not removed)
// and then asks the update library manager to tidy the update library.
func (ta *TufAutoupdater) tidyLibrary() {
	for _, binary := range binaries {
		// Get the current running version to preserve it when tidying the available updates
		currentVersion, err := ta.currentRunningVersion(binary)
		if err != nil {
			ta.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not get current running version",
				"binary", binary,
				"err", err,
			)
			continue
		}

		ta.libraryManager.TidyLibrary(binary, currentVersion)
	}
}

// currentRunningVersion returns the current running version of the given binary.
// It will perform retries for osqueryd.
func (ta *TufAutoupdater) currentRunningVersion(binary autoupdatableBinary) (string, error) {
	switch binary {
	case binaryLauncher:
		launcherVersion := version.Version().Version
		if launcherVersion == "unknown" {
			return "", errors.New("unknown launcher version")
		}
		return launcherVersion, nil
	case binaryOsqueryd:
		// first verify that the osqueryd binary exists. if it doesn't, there's no reason to wait
		// for initialization below
		if ta.knapsack != nil {
			latestOsqdPath := ta.knapsack.LatestOsquerydPath(context.TODO())
			if _, statErr := os.Stat(latestOsqdPath); os.IsNotExist(statErr) {
				return "", fmt.Errorf("finding current running version: osqueryd binary does not exist at `%s`", latestOsqdPath)
			}
		}

		// The osqueryd client may not have initialized yet, so retry the version
		// check a couple times before giving up
		osquerydVersionCheckRetries := 5
		var err error
		for i := 0; i < osquerydVersionCheckRetries; i += 1 {
			var resp []map[string]string
			resp, err = ta.osquerier.Query("SELECT version FROM osquery_info;")
			if err == nil && len(resp) > 0 {
				if osquerydVersion, ok := resp[0]["version"]; ok {
					return osquerydVersion, nil
				}
			}
			err = fmt.Errorf("error querying for osquery_info: %w; rows returned: %d", err, len(resp))

			time.Sleep(ta.osquerierRetryInterval)
		}
		return "", err
	default:
		return "", fmt.Errorf("cannot determine current running version for unexpected binary %s", binary)
	}
}

// checkForUpdate fetches latest metadata from the TUF server, then checks to see if there's
// a new release that we should download. If so, it will add the release to our updates library.
func (ta *TufAutoupdater) checkForUpdate(ctx context.Context, binariesToCheck []autoupdatableBinary) error {
	ctx, span := observability.StartSpan(ctx, "binaries", fmt.Sprintf("%+v", binariesToCheck))
	defer span.End()

	ta.updateLock.Lock()
	defer ta.updateLock.Unlock()

	// Skip autoupdating when Windows is sleeping. The Service Manager often has trouble with starting up
	// launcher while sleeping, so skipping the check is our safest option to keep launcher running
	// and functional.
	if ta.knapsack.InModernStandby() {
		ta.slogger.Log(ctx, slog.LevelInfo,
			"skipping autoupdate while in modern standby",
		)
		return nil
	}

	// Attempt an update a couple times before returning an error -- sometimes we just hit caching issues.
	errs := make([]error, 0)
	successfulUpdate := false
	updateTryCount := 3
	for i := 0; i < updateTryCount; i += 1 {
		_, err := ta.metadataClient.Update()
		if err == nil {
			successfulUpdate = true
			break
		}

		errs = append(errs, fmt.Errorf("try %d: %w", i, err))
	}
	if !successfulUpdate {
		return fmt.Errorf("could not update metadata after %d tries: %+v", updateTryCount, errs)
	}

	// Find the newest release for our channel
	targets, err := ta.metadataClient.Targets()
	if err != nil {
		return fmt.Errorf("could not get complete list of targets: %w", err)
	}

	// Check for and download any new releases that are available
	updatesDownloaded := make(map[autoupdatableBinary]string)
	updateErrors := make([]error, 0)
	for _, binary := range binariesToCheck {
		downloadedUpdateVersion, err := ta.downloadUpdate(binary, targets)
		if err != nil {
			updateErrors = append(updateErrors, fmt.Errorf("could not download update for %s: %w", binary, err))
		}

		if downloadedUpdateVersion != "" {
			ta.slogger.Log(ctx, slog.LevelInfo,
				"update downloaded",
				"binary", binary,
				"binary_version", downloadedUpdateVersion,
			)
			updatesDownloaded[binary] = versionFromTarget(binary, downloadedUpdateVersion)
		}

	}

	// If an update failed, save the error
	if len(updateErrors) > 0 {
		return fmt.Errorf("could not download updates: %+v", updateErrors)
	}

	// If launcher was updated, we want to exit and reload
	if updatedVersion, ok := updatesDownloaded[binaryLauncher]; ok {
		// Only reload if we're not using a localdev path
		if ta.knapsack.LocalDevelopmentPath() == "" {
			ta.slogger.Log(ctx, slog.LevelInfo,
				"launcher updated -- exiting to load new version",
				"new_binary_version", updatedVersion,
			)
			ta.signalRestart <- NewLauncherReloadNeededErr(updatedVersion)
			return nil
		}
	}

	// For non-launcher binaries (i.e. osqueryd), call any reload functions we have saved
	for binary, newBinaryVersion := range updatesDownloaded {
		if binary == binaryLauncher {
			continue
		}

		if restart, ok := ta.restartFuncs[binary]; ok {
			if err := restart(ctx); err != nil {
				ta.slogger.Log(ctx, slog.LevelWarn,
					"failed to restart binary after update",
					"binary", binary,
					"new_binary_version", newBinaryVersion,
					"err", err,
				)
				continue
			}

			ta.slogger.Log(ctx, slog.LevelInfo,
				"restarted binary after update",
				"binary", binary,
				"new_binary_version", newBinaryVersion,
			)
		}
	}

	return nil
}

// downloadUpdate will download a new release for the given binary, if available from TUF
// and not already downloaded.
func (ta *TufAutoupdater) downloadUpdate(binary autoupdatableBinary, targets data.TargetFiles) (string, error) {
	target, targetMetadata, err := findTarget(context.Background(), binary, targets, ta.pinnedVersions[binary], ta.updateChannel, ta.slogger)
	if err != nil {
		return "", fmt.Errorf("could not find appropriate target: %w", err)
	}

	// Ensure we don't download duplicate versions
	var currentVersion string
	currentVersion, _ = ta.currentRunningVersion(binary)
	if currentVersion == versionFromTarget(binary, target) {
		return "", nil
	}

	if ta.libraryManager.Available(binary, target) {
		// The release is already available in the library but we don't know if we're running it --
		// err on the side of not restarting.
		if currentVersion == "" {
			return "", nil
		}

		// We're choosing to run a local build, so we don't need to restart to run a new download.
		if binary == binaryLauncher && ta.knapsack.LocalDevelopmentPath() != "" {
			return "", nil
		}

		// The release is already available in the library and it's not our current running version --
		// return the version to signal for a restart.
		return target, nil
	}

	// We haven't yet downloaded this release -- download it
	if err := ta.libraryManager.AddToLibrary(binary, currentVersion, target, targetMetadata); err != nil {
		return "", fmt.Errorf("could not add target %s for binary %s to library: %w", target, binary, err)
	}

	return target, nil
}

// findTarget selects the appropriate target from `targets` for the given binary, using the pinned version (if set)
// and otherwise selecting the correct release for the given channel.
func findTarget(ctx context.Context, binary autoupdatableBinary, targets data.TargetFiles, pinnedVersion string, channel string, slogger *slog.Logger) (string, data.TargetFileMeta, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	if pinnedVersion != "" {
		target, targetMetadata, err := findTargetByVersion(ctx, binary, targets, pinnedVersion)
		if err == nil {
			// Binary version found
			return target, targetMetadata, nil
		}
		slogger.Log(ctx, slog.LevelWarn,
			"could not find target for version, falling back to release version",
			"pinned_version", pinnedVersion,
			"binary", binary,
			"err", err,
		)
	}

	// Either there isn't a pinned version, or the pinned version couldn't be found --
	// find the release target for the given channel instead.
	return findRelease(ctx, binary, targets, channel)
}

// findTargetByVersion selects the appropriate target from `targets` for the given binary and version.
func findTargetByVersion(ctx context.Context, binary autoupdatableBinary, targets data.TargetFiles, binaryVersion string) (string, data.TargetFileMeta, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	targetNameForVersion := path.Join(string(binary), runtime.GOOS, PlatformArch(), fmt.Sprintf("%s-%s.tar.gz", binary, binaryVersion))

	for targetName, target := range targets {
		if targetName != targetNameForVersion {
			continue
		}

		return filepath.Base(targetName), target, nil
	}
	return "", data.TargetFileMeta{}, fmt.Errorf("could not find metadata for binary %s and version %s", binary, binaryVersion)
}

// findRelease checks the latest data from TUF (in `targets`) to see whether a new release
// has been published for the given channel. If it has, it returns the target for that release
// and its associated metadata.
func findRelease(ctx context.Context, binary autoupdatableBinary, targets data.TargetFiles, channel string) (string, data.TargetFileMeta, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	// First, find the target that the channel release file is pointing to
	var releaseTarget string
	targetReleaseFile := path.Join(string(binary), runtime.GOOS, PlatformArch(), channel, "release.json")
	for targetName, target := range targets {
		if targetName != targetReleaseFile {
			continue
		}

		// We found the release file that matches our OS and binary. Evaluate it
		// to see if we're on this latest version.
		var custom ReleaseFileCustomMetadata
		if err := json.Unmarshal(*target.Custom, &custom); err != nil {
			return "", data.TargetFileMeta{}, fmt.Errorf("could not unmarshal release file custom metadata: %w", err)
		}

		releaseTarget = custom.Target
		break
	}

	if releaseTarget == "" {
		return "", data.TargetFileMeta{}, fmt.Errorf("expected release file %s for binary %s to be in targets but it was not", targetReleaseFile, binary)
	}

	// Now, get the metadata for our release target
	for targetName, target := range targets {
		if targetName != releaseTarget {
			continue
		}

		return filepath.Base(releaseTarget), target, nil
	}

	return "", data.TargetFileMeta{}, fmt.Errorf("could not find metadata for release target %s for binary %s", releaseTarget, binary)
}

// PlatformArch returns the correct arch for the runtime OS. For now, since osquery doesn't publish an arm64 release,
// we use the universal binaries for darwin.
func PlatformArch() string {
	if runtime.GOOS == "darwin" {
		return "universal"
	}

	return runtime.GOARCH
}

// storeError saves errors that occur during the periodic check for updates, so that they
// can be queryable via the `kolide_tuf_autoupdater_errors` table.
func (ta *TufAutoupdater) storeError(autoupdateErr error) {
	timestamp := strconv.Itoa(int(time.Now().Unix()))
	if err := ta.store.Set([]byte(timestamp), []byte(autoupdateErr.Error())); err != nil {
		ta.slogger.Log(context.TODO(), slog.LevelError,
			"could not store autoupdater error",
			"err", err,
		)
	}
}

// cleanUpOldErrors removes all errors from our store that are more than a week old,
// so we only keep the most recent/salient errors.
func (ta *TufAutoupdater) cleanUpOldErrors() {
	// We want to delete all errors more than 1 week old
	errorTtl := 7 * 24 * time.Hour

	// Read through all keys in bucket to determine which ones are old enough to be deleted
	keysToDelete := make([][]byte, 0)
	if err := ta.store.ForEach(func(k, _ []byte) error {
		// Key is a timestamp
		ts, err := strconv.ParseInt(string(k), 10, 64)
		if err != nil {
			// Delete the corrupted key
			keysToDelete = append(keysToDelete, k)
			return nil
		}

		errorTimestamp := time.Unix(ts, 0)
		if errorTimestamp.Add(errorTtl).Before(time.Now()) {
			keysToDelete = append(keysToDelete, k)
		}

		return nil
	}); err != nil {
		ta.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not iterate over bucket items to determine which are expired",
			"err", err,
		)
	}

	// Delete all old keys
	if err := ta.store.Delete(keysToDelete...); err != nil {
		ta.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not delete old autoupdater errors from bucket",
			"err", err,
		)
	}
}

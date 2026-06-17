// Package tuf provides an autoupdater that uses our new TUF infrastructure,
// replacing the previous Notary-based implementation. It allows launcher to
// download new launcher and osqueryd binaries.
package tuf

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
	kolideatomic "github.com/kolide/launcher/v2/pkg/atomic"
	"github.com/kolide/launcher/v2/pkg/osquery/runsimple"
	client "github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
	"github.com/theupdateframework/go-tuf/data"
)

//go:embed assets/tuf/root.json
var rootJson []byte

// RootJSON returns the embedded TUF root.json for client initialization.
func RootJSON() []byte {
	return rootJson
}

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
	Target      string `json:"target"`
	PromoteTime int64  `json:"promote_time"`
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

//mockery:generate: true
//mockery:dir: "{{.InterfaceDir}}"
//mockery:filename: mock_librarian_test.go
//mockery:pkgname: tuf
//mockery:structname: Mocklibrarian
type librarian interface {
	Available(binary autoupdatableBinary, targetFilename string) bool
	AddToLibrary(binary autoupdatableBinary, currentVersion string, targetFilename string, targetMetadata data.TargetFileMeta) error
	TidyLibrary(binary autoupdatableBinary, currentVersion string)
}

type TufAutoupdater struct {
	metadataClient       *client.Client
	libraryManager       librarian
	osqueryTimeout       time.Duration
	knapsack             types.Knapsack
	updateChannel        string
	pinnedVersions       map[autoupdatableBinary]string        // maps the binaries to their pinned versions
	pinnedVersionGetters map[autoupdatableBinary]func() string // maps the binaries to the knapsack function to retrieve updated pinned versions
	initialDelayStart    time.Time                             // when the autoupdater was created
	initialDelayEnd      atomic.Value                          // stores time.Time for thread-safe access
	initialDelayTimer    *time.Timer                           // fires when the initial delay ends so deferred restarts can run
	updateLock           *sync.Mutex
	checkTicker          *time.Ticker
	interrupt            chan struct{}
	interrupted          atomic.Bool
	signalRestart        chan error
	slogger              *slog.Logger
	restartFuncs         map[autoupdatableBinary]func(context.Context) error
	calculatedSplayDelay *atomic.Int64          // the randomly selected delay within the download splay window
	osqueryHistory       types.OsqueryHistorian // used to determine the version of the currently-running osquery process
	// Restarts are gated by the initial delay: during the delay we still check for and download
	// updates, but defer the restart until the delay ends, recording the pending restart here.
	pendingRestartLauncherVersion *kolideatomic.String // launcher version awaiting restart (empty if none)
	pendingRestartOsquerydVersion *kolideatomic.String // osqueryd version awaiting restart (empty if none)
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

// WithOsqueryHistory provides the autoupdater with osquery instance history, so that it can
// determine the version of the currently-running osquery process.
func WithOsqueryHistory(osqueryHistory types.OsqueryHistorian) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		ta.osqueryHistory = osqueryHistory
	}
}

func NewTufAutoupdater(ctx context.Context, k types.Knapsack, metadataHttpClient *http.Client, mirrorHttpClient *http.Client,
	opts ...TufAutoupdaterOption) (*TufAutoupdater, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	startTime := time.Now()
	ta := &TufAutoupdater{
		knapsack:          k,
		interrupt:         make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
		signalRestart:     make(chan error, 10),    // We have a buffer so we don't block on sending to this channel
		updateChannel:     k.UpdateChannel(),
		initialDelayStart: startTime,
		pinnedVersions: map[autoupdatableBinary]string{
			binaryLauncher: k.PinnedLauncherVersion(), // empty string if not pinned
			binaryOsqueryd: k.PinnedOsquerydVersion(), // ditto
		},
		pinnedVersionGetters: map[autoupdatableBinary]func() string{
			binaryLauncher: func() string { return k.PinnedLauncherVersion() },
			binaryOsqueryd: func() string { return k.PinnedOsquerydVersion() },
		},
		updateLock:                    &sync.Mutex{},
		osqueryTimeout:                30 * time.Second,
		slogger:                       k.Slogger().With("component", "tuf_autoupdater"),
		restartFuncs:                  make(map[autoupdatableBinary]func(context.Context) error),
		calculatedSplayDelay:          &atomic.Int64{},
		pendingRestartLauncherVersion: kolideatomic.NewString(""),
		pendingRestartOsquerydVersion: kolideatomic.NewString(""),
	}

	// Set initial delay end time atomically (calculated from start time)
	ta.initialDelayEnd.Store(startTime.Add(k.AutoupdateInitialDelay()))

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
	ta.knapsack.RegisterChangeObserver(ta, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay, keys.AutoupdateInterval, keys.AutoupdateInitialDelay)

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
	// For now, tidy the library on startup. In the future, we will tidy the library
	// earlier, after version selection.
	ta.tidyLibrary()

	ta.checkTicker = time.NewTicker(ta.knapsack.AutoupdateInterval())
	defer ta.checkTicker.Stop()

	// We begin checking for and downloading updates immediately, but we defer any restart
	// until the initial delay has elapsed. This timer fires when the initial delay ends so
	// that we can perform any restart that was deferred during the delay.
	ta.initialDelayTimer = time.NewTimer(time.Until(ta.initialDelayEnd.Load().(time.Time)))
	defer ta.initialDelayTimer.Stop()

	for {
		ta.slogger.Log(context.TODO(), slog.LevelInfo,
			"checking for updates",
		)
		// always allow our AutoupdateDownloadSplay delay during routine checks for autoupdates
		if err := ta.checkForUpdate(context.TODO(), binaries, true, false); err != nil {
			observability.AutoupdateFailureCounter.Add(context.TODO(), 1)
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
		case <-ta.initialDelayTimer.C:
			ta.slogger.Log(context.TODO(), slog.LevelInfo,
				"exiting initial delay",
			)

			// The initial delay has ended -- perform any restart that was deferred during the delay.
			if signalRestartErr := ta.restartIfPending(context.TODO()); signalRestartErr != nil {
				ta.slogger.Log(context.TODO(), slog.LevelInfo,
					"restarting launcher after deferred update at end of initial delay",
				)
				return signalRestartErr
			}
			continue
		case <-ta.checkTicker.C:
			continue
		}
	}
}

// restartIfPending performs any restart that was deferred during the initial delay. If a launcher
// restart is pending, it returns a LauncherReloadNeeded error so that Execute exits and the newly
// downloaded launcher is loaded. Otherwise, if an osqueryd restart is
// pending, it invokes the osqueryd restart function.
func (ta *TufAutoupdater) restartIfPending(ctx context.Context) error {
	if pendingVersion := ta.pendingRestartLauncherVersion.Load(); pendingVersion != "" {
		ta.pendingRestartLauncherVersion.Store("")
		ta.slogger.Log(ctx, slog.LevelInfo,
			"performing deferred launcher restart after initial delay",
			"new_binary_version", pendingVersion,
		)
		return NewLauncherReloadNeededErr(pendingVersion)
	}

	if pendingVersion := ta.pendingRestartOsquerydVersion.Load(); pendingVersion != "" {
		restart, ok := ta.restartFuncs[binaryOsqueryd]
		if !ok {
			return nil
		}

		ta.pendingRestartOsquerydVersion.Store("")

		ta.slogger.Log(ctx, slog.LevelInfo,
			"performing deferred osqueryd restart after initial delay",
			"new_binary_version", pendingVersion,
		)
		if err := restart(ctx); err != nil {
			ta.slogger.Log(ctx, slog.LevelWarn,
				"failed to restart osqueryd after deferred update at end of initial delay",
				"err", err,
			)
		}
	}

	return nil
}

func (ta *TufAutoupdater) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if ta.interrupted.Swap(true) {
		return
	}

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

	initialDelayEnd := ta.initialDelayEnd.Load().(time.Time)
	if time.Now().Before(initialDelayEnd) && !updateRequest.BypassInitialDelay {
		ta.slogger.Log(ctx, slog.LevelWarn,
			"received update request during initial delay, discarding",
			"initial_delay_end", initialDelayEnd.UTC().Format(time.RFC3339),
		)
		// We don't return an error because there's no need for the actionqueue to retry this request --
		// we're going to perform an autoupdate check on the next interval anyway.
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

	// do not allow AutoupdateDownloadSplay delay during autoupdate now requests
	if err := ta.checkForUpdate(ctx, binariesToUpdate, false, updateRequest.BypassInitialDelay); err != nil {
		observability.AutoupdateFailureCounter.Add(ctx, 1)
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

	// check if our autoupdate download splay has changed- if so,
	// reset our internally stored value and our calculated splay delay so
	// that it will be recalculated next time it is required
	if slices.Contains(flagKeys, keys.AutoupdateDownloadSplay) {
		ta.calculatedSplayDelay.Store(0)
	}

	// Check for AutoupdateInitialDelay changes
	if slices.Contains(flagKeys, keys.AutoupdateInitialDelay) {
		currentDelayEnd := ta.initialDelayEnd.Load().(time.Time)
		if time.Now().Before(currentDelayEnd) {
			newDelay := ta.knapsack.AutoupdateInitialDelay()
			// Calculate the new delay end time from the original start time
			newDelayEnd := ta.initialDelayStart.Add(newDelay)
			ta.initialDelayEnd.Store(newDelayEnd)
			// Keep the timer that triggers deferred restarts in sync with the new delay end.
			if ta.initialDelayTimer != nil {
				ta.initialDelayTimer.Reset(time.Until(newDelayEnd))
			}
			ta.slogger.Log(ctx, slog.LevelInfo,
				"autoupdate initial delay changed while in delay period",
				"old_delay_end", currentDelayEnd,
				"new_delay_end", newDelayEnd,
			)
		}
	}

	// Check for AutoupdateInterval changes
	autoupdateIntervalChanged := false
	if slices.Contains(flagKeys, keys.AutoupdateInterval) {
		newInterval := ta.knapsack.AutoupdateInterval()
		if ta.checkTicker != nil {
			ta.slogger.Log(ctx, slog.LevelInfo,
				"autoupdate interval changed, resetting ticker",
				"new_interval", newInterval,
			)
			ta.checkTicker.Reset(newInterval)
			autoupdateIntervalChanged = true
		}
	}

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

	// No updates, or interval did not change
	if len(binariesToCheckForUpdate) == 0 && !autoupdateIntervalChanged {
		return
	}

	// We're in the initial delay -- don't perform an update yet
	if time.Now().Before(ta.initialDelayEnd.Load().(time.Time)) {
		return
	}

	// If only the interval changed and no binaries need updating, check all binaries
	if len(binariesToCheckForUpdate) == 0 && autoupdateIntervalChanged {
		binariesToCheckForUpdate = binaries
	}

	// At least one binary requires a recheck or the interval changed -- perform that now
	// do not allow AutoupdateDownloadSplay delay when responding to flag changes
	if err := ta.checkForUpdate(ctx, binariesToCheckForUpdate, false, false); err != nil {
		observability.AutoupdateFailureCounter.Add(ctx, 1)
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
		// If possible, we pull the currently-running osquery version from the osquery history,
		// recorded by our current osquery instance after startup. We prefer this over `LatestOsquerydPath`
		// because that path is resolved against the TUF metadata we just refreshed, so during
		// a rollback it points to the rollback target that already exists in the update library --
		// leading the autoupdater to incorrectly conclude that it's already running the appropriate version
		// and not perform the osquery restart. This leaves us running the stale version until launcher
		// or osquery restarts independently.
		if ta.osqueryHistory != nil {
			if stats, err := ta.osqueryHistory.LatestInstanceStats(types.DefaultEnrollmentID); err == nil {
				if runningVersion, ok := stats["version"]; ok && runningVersion != "" {
					return runningVersion, nil
				}
			}
		}

		// Fall back to querying the binary directly -- for example, if there's no history
		// because osquery hasn't started up yet on a fresh install.
		// Query via runsimple, instead of osquery client, to avoid any socket contention.
		ctx, cancel := context.WithTimeout(context.Background(), ta.osqueryTimeout)
		defer cancel()

		var output bytes.Buffer
		osquerydPath := ta.knapsack.LatestOsquerydPath(ctx)
		osqVersionProc, err := runsimple.NewOsqueryProcess(osquerydPath, runsimple.WithStdout(&output))
		if err != nil {
			return "", fmt.Errorf("creating runsimple process to query for osqueryd version: %w", err)
		}

		if err := osqVersionProc.RunVersion(ctx); err != nil {
			return "", fmt.Errorf("running runsimple to query for osqueryd version: %w", err)
		}

		// Output looks like `osquery version x.y.z`, so split on `version` and return the last part of the string
		outputStr := strings.TrimSpace(output.String())
		parts := strings.SplitAfter(outputStr, "version")
		if len(parts) < 2 {
			return "", fmt.Errorf("malformed osqueryd version output %s", outputStr)
		}
		osquerydVersion := strings.TrimSpace(parts[len(parts)-1])

		return osquerydVersion, nil
	default:
		return "", fmt.Errorf("cannot determine current running version for unexpected binary %s", binary)
	}
}

// checkForUpdate fetches latest metadata from the TUF server, then checks to see if there's
// a new release that we should download. If so, it will add the release to our updates library.
// If allowDelay is set to false, our knapsack.AutoupdateDownloadSplay will be ignored and the update
// will be downloaded immediately, regardless of promotion time.
// If bypassInitialDelay is false and we are still within the initial delay, any required restart is
// deferred (recorded as pending) rather than performed immediately.
func (ta *TufAutoupdater) checkForUpdate(ctx context.Context, binariesToCheck []autoupdatableBinary, allowSplayDelay bool, bypassInitialDelay bool) error {
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
		downloadedUpdateVersion, err := ta.downloadUpdate(binary, targets, allowSplayDelay)
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

	// Determine whether we should defer any restart because we're still within the initial delay.
	// During the initial delay we still download updates, but we hold off on restarting so that we
	// don't restart immediately after installation. The deferred restart is performed once the delay ends.
	inInitialDelay := !bypassInitialDelay && time.Now().Before(ta.initialDelayEnd.Load().(time.Time))

	// If launcher was updated, we want to exit and reload
	if updatedVersion, ok := updatesDownloaded[binaryLauncher]; ok {
		// Only reload if we're not using a localdev path
		if ta.knapsack.LocalDevelopmentPath() == "" {
			if inInitialDelay {
				ta.pendingRestartLauncherVersion.Store(updatedVersion)
				ta.slogger.Log(ctx, slog.LevelInfo,
					"launcher updated during initial delay -- deferring restart until delay ends",
					"new_binary_version", updatedVersion,
				)

				// it is okay to return early here- even if there was also an osquery update at the exact same time
				// we would pick it up when we leave the initial delay and restart launcher, it is cleaner to
				// get the latest versions of both binaries that way anyway
				return nil
			}

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

		if inInitialDelay {
			ta.pendingRestartOsquerydVersion.Store(newBinaryVersion)
			ta.slogger.Log(ctx, slog.LevelInfo,
				"binary updated during initial delay -- deferring restart until delay ends",
				"binary", binary,
				"new_binary_version", newBinaryVersion,
			)
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
// and not already downloaded. If allowDelay is true, the download may be delayed according to
// the promotion time and the knapsack.AutoupdateDownloadSplay.
func (ta *TufAutoupdater) downloadUpdate(binary autoupdatableBinary, targets data.TargetFiles, allowDelay bool) (string, error) {
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

	// If the release is already available in our update library, there's no need to perform a download --
	// we can immediately return to load the newly-selected version.
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
		ta.slogger.Log(context.TODO(), slog.LevelInfo,
			"update is already available in library",
			"binary", binary,
			"target", target,
		)
		return target, nil
	}

	// Determine whether we should skip this check cycle if delaying the download
	if allowDelay && ta.shouldDelayDownload(binary, targets) {
		return "", nil
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
		if target.Custom == nil {
			return "", data.TargetFileMeta{}, fmt.Errorf("release file for %s missing custom metadata", binary)
		}
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

// findReleasePromoteTime extracts the promotion timestamp from the release file metadata for a given binary and channel.
// It searches the TUF targets for the appropriate release.json file based on the binary, OS, architecture, and channel,
// then unmarshals the custom metadata to retrieve the PromoteTime field.
// Returns the Unix timestamp of when the release was promoted, or 0 if the release file is not found or cannot be parsed.
func findReleasePromoteTime(ctx context.Context, binary autoupdatableBinary, targets data.TargetFiles, channel string) int64 {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	// find the metadata for the channel release file - this will include the promote_time
	targetReleaseFile := path.Join(string(binary), runtime.GOOS, PlatformArch(), channel, "release.json")
	for targetName, target := range targets {
		if targetName != targetReleaseFile {
			continue
		}

		if target.Custom == nil {
			return 0
		}
		var custom ReleaseFileCustomMetadata
		if err := json.Unmarshal(*target.Custom, &custom); err != nil {
			return 0
		}

		return custom.PromoteTime
	}

	return 0
}

// PlatformArch returns the correct arch for the runtime OS. For now, since osquery doesn't publish an arm64 release,
// we use the universal binaries for darwin.
func PlatformArch() string {
	return ArchForPlatform(runtime.GOOS, runtime.GOARCH)
}

// ArchForPlatform returns the TUF arch string for the given platform.
// Darwin uses "universal"; others use the provided arch.
func ArchForPlatform(platform, arch string) string {
	if platform == "darwin" {
		return "universal"
	}
	return arch
}

// shouldDelayDownload determines whether to delay downloading an update based on our AutoupdateDownloadSplay mechanism.
// It returns false (no delay) if:
// - AutoupdateDownloadSplay is disabled (set to 0s)
// - the release promote time cannot be determined or is unset
// - the promotion happened longer ago than the configured splay duration
// Otherwise, it uses a randomly selected delay offset within the splay window,
// returning true if the current time is before the calculated delay cutoff.
func (ta *TufAutoupdater) shouldDelayDownload(binary autoupdatableBinary, targets data.TargetFiles) bool {
	// if the splay is disabled, we should always download immediately
	if ta.knapsack.AutoupdateDownloadSplay() == 0 {
		return false
	}

	slogger := ta.slogger.With(
		"download_splay_minutes", ta.knapsack.AutoupdateDownloadSplay().Minutes(),
		"binary", binary,
		"update_channel", ta.updateChannel,
	)

	releasePromotedAt := findReleasePromoteTime(context.TODO(), binary, targets, ta.updateChannel)
	// if for any reason we can't determine the promote time, we should download immediately.
	// this also covers the case where we have not published a promote time for whatever reason
	if releasePromotedAt == 0 {
		slogger.Log(context.TODO(), slog.LevelDebug, "no release promotion time found, will not delay download")
		return false
	}

	promoteStart := time.Unix(releasePromotedAt, 0)
	// if promotion happened greater than our max splay threshold, we should download immediately
	if time.Since(promoteStart) > ta.knapsack.AutoupdateDownloadSplay() {
		slogger.Log(context.TODO(), slog.LevelDebug,
			"promote start was longer ago than download splay, will not delay download",
			"promote_start", promoteStart,
		)
		return false
	}

	splayDelaySeconds := ta.getSplayDelaySeconds()
	delayCutoffTime := time.Unix(releasePromotedAt+splayDelaySeconds, 0)
	slogger.Log(context.TODO(), slog.LevelInfo,
		"release promoted within splay time, determining download eligibility",
		"promote_start", promoteStart.UTC(),
		"delay_seconds", splayDelaySeconds,
		"delay_cutoff", delayCutoffTime.UTC(),
		"delay_minutes_from_now", time.Until(delayCutoffTime).Minutes(),
		"will_delay", time.Now().Before(delayCutoffTime),
	)

	// we should delay unless the current time is after the delay cutoff selected
	return time.Now().Before(delayCutoffTime)
}

// getSplayDelaySeconds returns a random delay value in seconds to indicate how long we should
// wait after promotion_time before downloading the update.
// This value is stable across a single launcher run- it uses a cached value if already calculated,
// otherwise generates a new random value between 0 and the configured AutoupdateDownloadSplay duration in seconds.
func (ta *TufAutoupdater) getSplayDelaySeconds() int64 {
	currentValue := ta.calculatedSplayDelay.Load()
	if currentValue != 0 {
		return currentValue
	}

	maxSplayValue := int64(ta.knapsack.AutoupdateDownloadSplay().Seconds())
	// we should never get here but no need to generate a random number if splay is disabled
	if maxSplayValue == 0 {
		return 0
	}
	// set a minimum of 1 second to ensure we don't set autoupdateDelaySplay to zero value
	// and regenerate a new value on subsequent calls
	newSplayValue := rand.Int63n(maxSplayValue) + 1
	ta.calculatedSplayDelay.Store(newSplayValue)
	return newSplayValue
}

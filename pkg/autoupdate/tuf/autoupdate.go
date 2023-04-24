package tuf

// This new autoupdater points to our new TUF infrastructure, and will eventually supersede
// the legacy `Updater` in pkg/autoupdate that points to Notary.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
	client "github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
	"github.com/theupdateframework/go-tuf/data"
)

//go:embed assets/tuf/root.json
var rootJson []byte

// Configuration defaults
const (
	DefaultTufServer = "https://tuf.kolide.com"
	defaultChannel   = "stable"
	tufDirectoryName = "tuf"
)

// Binaries handled by autoupdater
type autoupdatableBinary string

const (
	binaryLauncher autoupdatableBinary = "launcher"
	binaryOsqueryd autoupdatableBinary = "osqueryd"
)

var binaries = []autoupdatableBinary{binaryLauncher, binaryOsqueryd}

type ReleaseFileCustomMetadata struct {
	Target string `json:"target"`
}

type librarian interface {
	Available(binary autoupdatableBinary, targetFilename string) bool
	AddToLibrary(binary autoupdatableBinary, targetFilename string, targetMetadata data.TargetFileMeta) error
	TidyLibrary()
}

type TufAutoupdater struct {
	metadataClient *client.Client
	libraryManager librarian
	channel        string
	checkInterval  time.Duration
	store          types.KVStore // stores autoupdater errors for kolide_tuf_autoupdater_errors table
	interrupt      chan struct{}
	logger         log.Logger
}

type TufAutoupdaterOption func(*TufAutoupdater)

func WithLogger(logger log.Logger) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		ta.logger = log.With(logger, "component", "tuf_autoupdater")
	}
}

func WithChannel(channel string) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		ta.channel = channel
	}
}

func WithUpdateCheckInterval(checkInterval time.Duration) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		ta.checkInterval = checkInterval
	}
}

func NewTufAutoupdater(metadataUrl, rootDirectory string, updateDirectory string, metadataHttpClient *http.Client,
	mirrorUrl string, mirrorHttpClient *http.Client, store types.KVStore, osquerier querier, opts ...TufAutoupdaterOption) (*TufAutoupdater, error) {
	ta := &TufAutoupdater{
		channel:       defaultChannel,
		interrupt:     make(chan struct{}),
		checkInterval: 60 * time.Second,
		store:         store,
		logger:        log.NewNopLogger(),
	}

	for _, opt := range opts {
		opt(ta)
	}

	var err error
	ta.metadataClient, err = initMetadataClient(rootDirectory, metadataUrl, metadataHttpClient)
	if err != nil {
		return nil, fmt.Errorf("could not init metadata client: %w", err)
	}

	// If the update directory wasn't set by a flag, use the default location of <launcher root>/updates.
	if updateDirectory == "" {
		updateDirectory = filepath.Join(rootDirectory, "updates")
	}
	ta.libraryManager, err = newUpdateLibraryManager(mirrorUrl, mirrorHttpClient, updateDirectory, osquerier, ta.logger)
	if err != nil {
		return nil, fmt.Errorf("could not init update library manager: %w", err)
	}

	return ta, nil
}

// initMetadataClient sets up a TUF client with our validated root metadata, prepared to fetch updates
// from our TUF server.
func initMetadataClient(rootDirectory, metadataUrl string, metadataHttpClient *http.Client) (*client.Client, error) {
	// Set up the local TUF directory for our TUF client
	localTufDirectory := LocalTufDirectory(rootDirectory)
	if err := os.MkdirAll(localTufDirectory, 0750); err != nil {
		return nil, fmt.Errorf("could not make local TUF directory %s: %w", localTufDirectory, err)
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

// Execute is the TufAutoupdater run loop. It periodically checks to see if a new release
// has been published; less frequently, it removes old/outdated TUF errors from the bucket
// we store them in.
func (ta *TufAutoupdater) Execute() (err error) {
	// For now, tidy the library on startup. In the future, we will tidy the library
	// earlier, after version selection.
	ta.libraryManager.TidyLibrary()

	checkTicker := time.NewTicker(ta.checkInterval)
	defer checkTicker.Stop()
	cleanupTicker := time.NewTicker(12 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-checkTicker.C:
			if err := ta.checkForUpdate(); err != nil {
				ta.storeError(err)
				level.Debug(ta.logger).Log("msg", "error checking for update", "err", err)
			}
		case <-cleanupTicker.C:
			ta.cleanUpOldErrors()
		case <-ta.interrupt:
			level.Debug(ta.logger).Log("msg", "received interrupt, stopping")
			return nil
		}
	}
}

func (ta *TufAutoupdater) Interrupt(_ error) {
	ta.interrupt <- struct{}{}
}

// checkForUpdate fetches latest metadata from the TUF server, then checks to see if there's
// a new release that we should download. If so, it will add the release to our updates library.
func (ta *TufAutoupdater) checkForUpdate() error {
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
	updatesDownloaded := make([]bool, len(binaries))
	updateErrors := make([]error, 0)
	for i, binary := range binaries {
		downloadedUpdateVersion, err := ta.downloadUpdate(binary, targets)
		if err != nil {
			updateErrors = append(updateErrors, fmt.Errorf("could not download update for %s: %w", binary, err))
		}

		if downloadedUpdateVersion != "" {
			level.Debug(ta.logger).Log("msg", "update downloaded", "binary", binary, "version", downloadedUpdateVersion)
			updatesDownloaded[i] = true
		} else {
			updatesDownloaded[i] = false
		}

	}

	// If an update failed, save the error
	if len(updateErrors) > 0 {
		return fmt.Errorf("could not download updates: %+v", updateErrors)
	}

	for _, updateDownloaded := range updatesDownloaded {
		if updateDownloaded {
			// In the future, we would restart or re-launch the binary with the new version
			level.Debug(ta.logger).Log("msg", "at least one update downloaded")
			break
		}
	}

	return nil
}

// downloadUpdate will download a new release for the given binary, if available from TUF
// and not already downloaded.
func (ta *TufAutoupdater) downloadUpdate(binary autoupdatableBinary, targets data.TargetFiles) (string, error) {
	release, releaseMetadata, err := ta.findRelease(binary, targets)
	if err != nil {
		return "", fmt.Errorf("could not find release: %w", err)
	}

	if ta.libraryManager.Available(binary, release) {
		return "", nil
	}

	if err := ta.libraryManager.AddToLibrary(binary, release, releaseMetadata); err != nil {
		return "", fmt.Errorf("could not add release %s for binary %s to library: %w", release, binary, err)
	}

	return release, nil
}

// findRelease checks the latest data from TUF (in `targets`) to see whether a new release
// has been published for our channel. If it has, it returns the target for that release
// and its associated metadata.
func (ta *TufAutoupdater) findRelease(binary autoupdatableBinary, targets data.TargetFiles) (string, data.TargetFileMeta, error) {
	// First, find the target that the channel release file is pointing to
	var releaseTarget string
	targetReleaseFile := fmt.Sprintf("%s/%s/%s/release.json", binary, runtime.GOOS, ta.channel)
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

	return "", data.TargetFileMeta{}, fmt.Errorf("could not find metadata for release target %s for binary %s", targetReleaseFile, binary)
}

// storeError saves errors that occur during the periodic check for updates, so that they
// can be queryable via the `kolide_tuf_autoupdater_errors` table.
func (ta *TufAutoupdater) storeError(autoupdateErr error) {
	timestamp := strconv.Itoa(int(time.Now().Unix()))
	if err := ta.store.Set([]byte(timestamp), []byte(autoupdateErr.Error())); err != nil {
		level.Debug(ta.logger).Log("msg", "could not store autoupdater error", "err", err)
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
		level.Debug(ta.logger).Log("msg", "could not iterate over bucket items to determine which are expired", "err", err)
	}

	// Delete all old keys
	if err := ta.store.Delete(keysToDelete...); err != nil {
		level.Debug(ta.logger).Log("msg", "could not delete old autoupdater errors from bucket", "err", err)
	}
}

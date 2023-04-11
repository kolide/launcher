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

func (ta *TufAutoupdater) Execute() (err error) {
	// For now, tidy the library on startup. In the future, we will tidy the library
	// earlier, after version selection.
	ta.libraryManager.TidyLibrary()

	checkTicker := time.NewTicker(ta.checkInterval)
	cleanupTicker := time.NewTicker(12 * time.Hour)

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

	// Find the newest release for our channel -- right now for logging purposes only
	targets, err := ta.metadataClient.Targets()
	if err != nil {
		return fmt.Errorf("could not get complete list of targets: %w", err)
	}

	for _, binary := range binaries {
		if err := ta.findRelease(binary, targets); err != nil {
			return fmt.Errorf("could not find release: %w", err)
		}
	}

	return nil
}

func (ta *TufAutoupdater) findRelease(binary autoupdatableBinary, targets data.TargetFiles) error {
	targetReleaseFile := fmt.Sprintf("%s/%s/%s/release.json", binary, runtime.GOOS, ta.channel)
	for targetName, target := range targets {
		if targetName != targetReleaseFile {
			continue
		}

		// We found the release file that matches our OS and binary. Evaluate it
		// to see if we're on this latest version.
		var custom ReleaseFileCustomMetadata
		if err := json.Unmarshal(*target.Custom, &custom); err != nil {
			return fmt.Errorf("could not unmarshal release file custom metadata: %w", err)
		}

		targetToDownload, err := ta.metadataClient.Target(custom.Target)
		if err != nil {
			return fmt.Errorf("could not get TUF metadata for release %s: %w", custom.Target, err)
		}

		return ta.libraryManager.AddToLibrary(binary, filepath.Base(custom.Target), targetToDownload)
	}

	return fmt.Errorf("expected release file %s for binary %s to be in targets but it was not", targetReleaseFile, binary)
}

func (ta *TufAutoupdater) storeError(autoupdateErr error) {
	timestamp := strconv.Itoa(int(time.Now().Unix()))
	if err := ta.store.Set([]byte(timestamp), []byte(autoupdateErr.Error())); err != nil {
		level.Debug(ta.logger).Log("msg", "could not store autoupdater error", "err", err)
	}
}

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

package autoupdate

// This new autoupdater points to our new TUF infrastructure, and will eventually supersede
// the legacy `Updater` in autoupdate.go that points to Notary.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/version"
	legacytuf "github.com/kolide/updater/tuf"
	client "github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

//go:embed assets/tuf-dev/root.json
var rootJson []byte

const (
	DefaultTufServer = "https://tuf-devel.kolide.com"
)

type TufAutoupdater struct {
	metadataClient  *client.Client
	mirrorClient    *http.Client
	mirrorUrl       string
	binary          string
	operatingSystem string
	channel         UpdateChannel
	checkInterval   time.Duration
	errorCounter    []int64
	lock            sync.RWMutex
	interrupt       chan struct{}
	logger          log.Logger
}

type TufAutoupdaterOption func(*TufAutoupdater)

func WithTufLogger(logger log.Logger) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		ta.logger = log.With(logger, "component", "tuf_autoupdater")
	}
}

func WithChannel(channel UpdateChannel) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		ta.channel = channel
	}
}

func WithUpdateCheckInterval(checkInterval time.Duration) TufAutoupdaterOption {
	return func(ta *TufAutoupdater) {
		ta.checkInterval = checkInterval
	}
}

func NewTufAutoupdater(metadataUrl, mirrorUrl, binaryPath, rootDirectory string, opts ...TufAutoupdaterOption) (*TufAutoupdater, error) {
	binaryName := filepath.Base(binaryPath)

	// Set up the local TUF directory for our TUF client -- a dev repo, to be replaced once we move to production
	localTufDirectory := filepath.Join(rootDirectory, fmt.Sprintf("%s-tuf-dev", strings.TrimSuffix(binaryName, ".exe")))
	if err := os.MkdirAll(localTufDirectory, 0750); err != nil {
		return nil, fmt.Errorf("could not make local TUF directory %s: %w", localTufDirectory, err)
	}

	// Set up our local store i.e. point to the directory in our filesystem
	localStore, err := filejsonstore.NewFileJSONStore(localTufDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not initialize local TUF store: %w", err)
	}

	// Set up our remote store i.e. tuf-devel.kolide.com
	remoteOpts := client.HTTPRemoteOptions{
		MetadataPath: "/repository",
	}
	remoteStore, err := client.HTTPRemoteStore(metadataUrl, &remoteOpts, http.DefaultClient)
	if err != nil {
		return nil, fmt.Errorf("could not initialize remote TUF store: %w", err)
	}

	metadataClient := client.NewClient(localStore, remoteStore)
	if err := metadataClient.Init(rootJson); err != nil {
		return nil, fmt.Errorf("failed to initialize TUF client with root JSON: %w", err)
	}

	ta := &TufAutoupdater{
		metadataClient:  metadataClient,
		mirrorClient:    http.DefaultClient,
		mirrorUrl:       mirrorUrl,
		binary:          binaryName,
		operatingSystem: runtime.GOOS,
		channel:         Stable,
		interrupt:       make(chan struct{}),
		checkInterval:   60 * time.Second,
		errorCounter:    make([]int64, 0), // For now, the error counter is a simple list of timestamps when errors occurred
		logger:          log.NewNopLogger(),
	}

	for _, opt := range opts {
		opt(ta)
	}

	return ta, nil
}

func (ta *TufAutoupdater) Run(opts ...legacytuf.Option) (stop func(), err error) {
	go ta.loop()

	return ta.stop, nil
}

func (ta *TufAutoupdater) RollingErrorCount() int {
	ta.lock.RLock()
	defer ta.lock.RUnlock()

	oneDayAgo := time.Now().Add(-24 * time.Hour).Unix()
	errorCount := 0
	for _, errorTimestamp := range ta.errorCounter {
		if errorTimestamp < oneDayAgo {
			continue
		}
		errorCount += 1
	}

	return errorCount
}

func (ta *TufAutoupdater) loop() error {
	checkTicker := time.NewTicker(ta.checkInterval)

	for {
		select {
		case <-checkTicker.C:
			if err := ta.checkForUpdate(); err != nil {
				ta.lock.Lock()
				ta.errorCounter = append(ta.errorCounter, time.Now().Unix())
				ta.lock.Unlock()

				level.Debug(ta.logger).Log("msg", "error checking for update", "err", err)
			}
		case <-ta.interrupt:
			level.Debug(ta.logger).Log("msg", "received interrupt, stopping")
			return nil
		}
	}
}

func (ta *TufAutoupdater) stop() {
	ta.interrupt <- struct{}{}
}

func (ta *TufAutoupdater) checkForUpdate() error {
	_, err := ta.metadataClient.Update()
	if err != nil {
		return fmt.Errorf("could not update metadata: %w", err)
	}

	// Find the newest release for our channel -- right now for logging purposes only
	targets, err := ta.metadataClient.Targets()
	if err != nil {
		return fmt.Errorf("could not get complete list of targets: %w", err)
	}

	targetReleaseFile := fmt.Sprintf("%s/%s/%s/release.json", strings.TrimSuffix(ta.binary, ".exe"), ta.operatingSystem, ta.channel)
	for targetName, target := range targets {
		if targetName != targetReleaseFile {
			continue
		}

		// We found the release file that matches our OS and binary. Evaluate it
		// to see if we're on this latest version.
		type releaseFileCustomMetadata struct {
			Target string `json:"target"`
		}

		var custom releaseFileCustomMetadata
		if err := json.Unmarshal(*target.Custom, &custom); err != nil {
			return fmt.Errorf("could not unmarshal release file custom metadata: %w", err)
		}

		level.Debug(ta.logger).Log(
			"msg", "checked most up-to-date release from TUF",
			"launcher_version", version.Version().Version,
			"release_version", ta.versionFromTarget(custom.Target),
			"channel", ta.channel,
		)

		break
	}

	return nil
}

func (ta *TufAutoupdater) versionFromTarget(target string) string {
	strippedBinary := strings.TrimSuffix(ta.binary, ".exe")

	// The target is in the form `launcher/linux/launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s/%s/%s-", strippedBinary, ta.operatingSystem, strippedBinary)

	return strings.TrimSuffix(strings.TrimPrefix(target, prefixToTrim), ".tar.gz")
}

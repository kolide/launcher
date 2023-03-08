package autoupdate

// This new autoupdater points to our new TUF infrastructure, and will eventually supersede
// the legacy `Updater` in autoupdate.go that points to Notary.

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/armon/go-metrics"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	legacytuf "github.com/kolide/updater/tuf"
	client "github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

//go:embed assets/tuf-new/root.json
var rootJson []byte

const (
	DefaultTufServer = "https://tuf-devel.kolide.com"
	errorCounterKey  = "tuf_errors"
)

type TufAutoupdater struct {
	metadataClient   *client.Client
	mirrorClient     *http.Client
	mirrorUrl        string
	binary           string
	channel          UpdateChannel
	stagingDirectory string
	updatesDirectory string
	checkInterval    time.Duration
	errorCounter     *metrics.InmemSink
	interrupt        chan struct{}
	logger           log.Logger
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
	// Ensure that the staging directory exists, creating it if not
	binaryName := filepath.Base(binaryPath)
	stagingDirectory := filepath.Join(rootDirectory, fmt.Sprintf("%s-staging", binaryName))
	if err := os.MkdirAll(stagingDirectory, 0755); err != nil {
		return nil, fmt.Errorf("could not make staging directory %s: %w", stagingDirectory, err)
	}

	// Ensure that the updates directory exists, creating it if not
	updatesDirectory := filepath.Join(FindBaseDir(binaryPath), fmt.Sprintf("%s-updates", binaryName))
	if err := os.MkdirAll(updatesDirectory, 0755); err != nil {
		return nil, fmt.Errorf("could not make updates directory %s: %w", updatesDirectory, err)
	}

	// Set up the local TUF directory for our TUF client
	localTufDirectory := filepath.Join(rootDirectory, fmt.Sprintf("%s-tuf-new", strings.TrimSuffix(binaryName, ".exe")))
	if err := os.MkdirAll(localTufDirectory, 0755); err != nil {
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

	// Set up our error tracker -- holds error counts per hour for the last 24 hours
	errorCounter := metrics.NewInmemSink(1*time.Hour, 24*time.Hour)
	metrics.NewGlobal(metrics.DefaultConfig("tuf_autoupdater"), errorCounter)

	ta := &TufAutoupdater{
		metadataClient:   metadataClient,
		mirrorClient:     http.DefaultClient,
		mirrorUrl:        mirrorUrl,
		binary:           binaryName,
		channel:          Stable,
		stagingDirectory: stagingDirectory,
		updatesDirectory: updatesDirectory,
		interrupt:        make(chan struct{}),
		checkInterval:    60 * time.Second,
		errorCounter:     errorCounter,
		logger:           log.NewNopLogger(),
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

func (ta *TufAutoupdater) ErrorCount() int {
	intervalMetrics := ta.errorCounter.Data()

	errorCount := 0
	for _, m := range intervalMetrics {
		if val, ok := m.Counters[errorCounterKey]; ok {
			errorCount += val.Count
		}
	}

	return errorCount
}

func (ta *TufAutoupdater) loop() error {
	checkTicker := time.NewTicker(ta.checkInterval)

	for {
		select {
		case <-checkTicker.C:
			if err := ta.checkForUpdate(); err != nil {
				ta.errorCounter.IncrCounter([]string{errorCounterKey}, 1)
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

	return nil
}

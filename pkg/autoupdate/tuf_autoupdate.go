package autoupdate

// This new autoupdater points to our new TUF infrastructure, and will eventually supersede
// the legacy `Updater` in autoupdate.go that points to Notary.

import (
	"context"
	"crypto/sha512"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/armon/go-metrics"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/version"
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
	finalizer        UpdateFinalizer
	restartChannel   chan os.Signal
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

func NewTufAutoupdater(metadataUrl, mirrorUrl, binaryPath, rootDirectory string, restartChannel chan os.Signal, opts ...TufAutoupdaterOption) (*TufAutoupdater, error) {
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
		restartChannel:   restartChannel,
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
	targets, err := ta.metadataClient.Update()
	if err != nil {
		return fmt.Errorf("could not update metadata: %w", err)
	}

	targetReleaseFile := fmt.Sprintf("%s/%s/%s/release.json", strings.TrimSuffix(ta.binary, ".exe"), runtime.GOOS, ta.channel)
	var newTarget string
	for targetName, target := range targets {
		if targetName != targetReleaseFile {
			continue
		}

		// We found the release file that matches our OS and binary. Evaluate it
		// to see if we have a new release to download.
		type releaseFileCustomMetadata struct {
			Target string `json:"target"`
		}

		var custom releaseFileCustomMetadata
		if err := json.Unmarshal(*target.Custom, &custom); err != nil {
			return fmt.Errorf("could not unmarshal release file custom metadata: %w", err)
		}

		newTargetVersion := ta.versionFromTarget(custom.Target)
		if newTargetVersion != version.Version().Version {
			newTarget = custom.Target
		}

		break
	}

	if newTarget == "" {
		// Nothing to do
		return nil
	}

	if err := ta.stageDownload(newTarget); err != nil {
		return fmt.Errorf("could not stage download: %w", err)
	}

	if err := ta.verifyStagedDownload(newTarget); err != nil {
		return fmt.Errorf("could not verify staged download: %w", err)
	}

	if err := ta.moveVerifiedUpdate(newTarget); err != nil {
		return fmt.Errorf("could not move verified update: %w", err)
	}

	ta.finalizeUpdate()

	return nil
}

func (ta *TufAutoupdater) versionFromTarget(target string) string {
	strippedBinary := strings.TrimSuffix(ta.binary, ".exe")

	// The target is in the form `launcher/linux/launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s/%s/%s-", strippedBinary, runtime.GOOS, strippedBinary)

	return strings.TrimSuffix(strings.TrimPrefix(target, prefixToTrim), ".tar.gz")
}

func (ta *TufAutoupdater) stageDownload(target string) error {
	stagedDownloadLocation := ta.downloadLocation(target)
	out, err := os.Create(stagedDownloadLocation)
	if err != nil {
		return fmt.Errorf("could not create file at %s: %w", stagedDownloadLocation, err)
	}
	defer out.Close()

	resp, err := ta.mirrorClient.Get(ta.mirrorUrl + fmt.Sprintf("/kolide/%s", target))
	if err != nil {
		return fmt.Errorf("could not make request to download target %s: %w", target, err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("could not write downloaded target %s to file %s: %w", target, stagedDownloadLocation, err)
	}

	return nil
}

func (ta *TufAutoupdater) verifyStagedDownload(target string) error {
	stagedDownloadLocation := ta.downloadLocation(target)
	digest, err := sha512Digest(stagedDownloadLocation)
	if err != nil {
		return fmt.Errorf("could not compute digest for target %s to verify it: %w", target, err)
	}

	fileInfo, err := os.Stat(stagedDownloadLocation)
	if err != nil {
		return fmt.Errorf("could not get info for downloaded file at %s: %w", stagedDownloadLocation, err)
	}

	tufRepoPath := filepath.Join(ta.binary, runtime.GOOS, target)
	if err := ta.metadataClient.VerifyDigest(digest, "sha512", fileInfo.Size(), tufRepoPath); err != nil {
		return fmt.Errorf("digest verification failed for target %s downloaded to %s: %w", target, stagedDownloadLocation, err)
	}

	return nil
}

func (ta *TufAutoupdater) moveVerifiedUpdate(target string) error {
	newUpdateDirectoryPath := filepath.Join(ta.updatesDirectory, strconv.FormatInt(time.Now().Unix(), 10))
	if err := os.MkdirAll(newUpdateDirectoryPath, 0755); err != nil {
		return fmt.Errorf("could not create directory %s for new update: %w", newUpdateDirectoryPath, err)
	}

	removeBrokenUpdateDir := func() {
		if err := os.RemoveAll(newUpdateDirectoryPath); err != nil {
			level.Debug(ta.logger).Log(
				"msg", "could not remove broken update directory",
				"update_dir", newUpdateDirectoryPath,
				"err", err,
			)
		}
	}

	fileToUntarAndMove := ta.downloadLocation(target)
	if err := fsutil.UntarBundle(filepath.Join(newUpdateDirectoryPath, ta.binary), fileToUntarAndMove); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not untar update %s to %s: %w", fileToUntarAndMove, newUpdateDirectoryPath, err)
	}

	// Make sure that the binary is executable
	outputBinary := filepath.Join(newUpdateDirectoryPath, ta.binary)
	if err := os.Chmod(outputBinary, 0755); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not set +x permissions on executable: %w", err)
	}

	// One final check
	if err := checkExecutable(context.TODO(), outputBinary, "--version"); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("executable check failed for %s (target %s): %w", outputBinary, target, err)
	}

	return nil
}

func (ta *TufAutoupdater) downloadLocation(target string) string {
	return filepath.Join(ta.stagingDirectory, runtime.GOOS, target)
}

func (ta *TufAutoupdater) finalizeUpdate() {
	err := ta.finalizer()
	if err == nil {
		// All set, no restart needed
		return
	}

	if IsLauncherRestartNeededErr(err) {
		level.Debug(ta.logger).Log("msg", "signaling for a full restart")
		ta.restartChannel <- os.Interrupt
		return
	}

	// Unexpected error -- trigger a restart
	level.Debug(ta.logger).Log("unexpected error when finalizing update, signaling for a full restart", "err", err)
	ta.restartChannel <- os.Interrupt
}

func sha512Digest(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("could not open file %s to calculate digest: %w", filename, err)
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("could not compute checksum for file %s: %w", filename, err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

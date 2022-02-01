// Package autoupdate provides a TUF Updater for the launcher and
// related binaries. This is abstracted across two packages, as well
// as main, making for a rather complex tangle.
//
// As different binaries need different strategies for restarting,
// there are several moving parts to this:
//
//    github.com/kolide/updater/tuf is kolide's client to The Update
//    Framework (also called notary). This library is based around
//    signed metadata. When the metadata changes, it will download the
//    linked file. (This idiom is a bit confusing, and a bit
//    limiting. It downloads on _metadata_ change, and not as a file
//    comparison)
//
//    tuf.NotificationHandler is responsible for moving the downloaded
//    binary into the desired location. It defined by this package,
//    and is passed to TUF as a function. It is also used by TUF as a
//    ad-hoc logging mechanism.
//
//    autoupdate.UpdateFinalizer is responsible for finalizing the
//    update. Eg: restarting the service appropriately. As it is
//    different per binary, it is defined by main, and passed in to
//    autoupdate.NewUpdater.
//
// Expected Usage
//
// For each binary that is being updated, main will create a rungroup
// actor.Actor, for the autouopdate.Updater. main is responsible for
// setting an appropriate finalizer.
//
// This actor is a wrapper around TUF. TUF will check at a specified
// interval for new metadata. If found, it will update the local
// metadata repo, and fetch a new binary.
//
// tuf will then call the updater's handler to move the resultant
// binary. And finally pass off to the finalizer.
//
// Testing
//
// While some functions can be unit tested, integration is tightly
// coupled to TUF. One of the simplest ways to test this, is by
// attaching to the `nightly` channel, and causing frequent updates.
//nolint:typecheck // parts of this come from bindata, so lint fails
package autoupdate

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/updater/tuf"
	"github.com/pkg/errors"
)

// UpdateChannel determines the TUF target for a Updater.
// The Default UpdateChannel is Stable.
type UpdateChannel string

const (
	Stable  UpdateChannel = "stable"
	Alpha                 = "alpha"
	Beta                  = "beta"
	Nightly               = "nightly"
)

const (
	DefaultMirror       = "https://dl.kolide.co"
	DefaultNotary       = "https://notary.kolide.co"
	DefaultNotaryPrefix = "kolide"
)

// Updater is a TUF autoupdater. It expects a tar.gz archive with an
// executable binary, which will be placed into an update area and
// spawned via appropriate platform mechanisms.
type Updater struct {
	binaryName         string          // What binary name on disk. This includes things like `.exe`
	strippedBinaryName string          // What is the binary name minus any extensions.
	bootstrapFn        func() error    // function to create the local TUF metadata
	finalizer          UpdateFinalizer // function that will "finalize" the update, by restarting the binary
	stagingPath        string          // Where should TUF stage the downloads
	updatesDirectory   string          // directory to store updates in
	target             string          // filename to download, passed to TUF.
	updateChannel      UpdateChannel   // Update channel (stable, nightly, etc)
	settings           *tuf.Settings   // tuf.Settings
	sigChannel         chan os.Signal  // channel for shutdown signaling
	client             *http.Client
	logger             log.Logger
}

// UpdateFinalizer is executed after the Updater updates a destination.
// The UpdateFinalizer is usually a function which will handle restarting the updated binary.
type UpdateFinalizer func() error

// NewUpdater creates a unstarted updater for a specific binary
// updated from a TUF mirror.
func NewUpdater(binaryPath, rootDirectory string, opts ...UpdaterOption) (*Updater, error) {
	// There's some chaos between windows and non-windows. In windows,
	// the binaryName ends in .exe, in posix it does not. So, a simple
	// TrimSuffix will handle stripping it. *However* this will break if
	// we add the extension. The suffix is inconistent. package-builder
	// has a lot of gnarly code around that. We may need to import it.
	binaryName := filepath.Base(binaryPath)
	strippedBinaryName := strings.TrimSuffix(binaryName, ".exe")
	tufRepoPath := filepath.Join(rootDirectory, fmt.Sprintf("%s-tuf", strippedBinaryName))

	settings := tuf.Settings{
		LocalRepoPath: tufRepoPath,
		NotaryURL:     DefaultNotary,
		GUN:           path.Join(DefaultNotaryPrefix, strippedBinaryName),
		MirrorURL:     DefaultMirror,
	}

	updater := Updater{
		settings:           &settings,
		updateChannel:      Stable,
		client:             http.DefaultClient,
		logger:             log.NewNopLogger(),
		finalizer:          func() error { return nil },
		strippedBinaryName: strippedBinaryName,
		binaryName:         binaryName,
	}

	// The staging directory is used as a temporary download
	// location for TUF. The updatesDirectory is used as a place
	// to hold newer binary versions. The updated binaries are
	// executated from this directory. We store the update
	// relatative to the binaryPath primarily so that command line
	// executions can find it, without needing to know where the
	// rootDirectory is. (it likely also helps uncommon noexec
	// cases)
	updater.stagingPath = filepath.Join(rootDirectory, fmt.Sprintf("%s-staging", binaryName))
	updater.updatesDirectory = filepath.Join(FindBaseDir(binaryPath), fmt.Sprintf("%s-updates", binaryName))

	// create TUF from local assets, but allow overriding with a no-op in tests.
	updater.bootstrapFn = updater.createLocalTufRepo

	for _, opt := range opts {
		opt(&updater)
	}

	if err := updater.setTargetPath(); err != nil {
		return nil, errors.Wrapf(err, "set updater target for destination %s", binaryPath)
	}

	if err := updater.bootstrapFn(); err != nil {
		return nil, errors.Wrap(err, "creating local TUF repo")
	}

	level.Debug(updater.logger).Log(
		"msg", "Created Updater",
		"binaryName", updater.binaryName,
		"stagingPath", updater.stagingPath,
		"updatesDirectory", updater.updatesDirectory,
	)

	return &updater, nil
}

// createLocalTufRepo bootstraps local TUF metadata from bindata
// assets. (TUF requires an initial starting repo)
func (u *Updater) createLocalTufRepo() error {
	if err := os.MkdirAll(u.settings.LocalRepoPath, 0755); err != nil {
		return errors.Wrapf(err, "mkdir LocalRepoPath (%s)", u.settings.LocalRepoPath)
	}
	localRepo := filepath.Base(u.settings.LocalRepoPath)
	assetPath := path.Join("pkg", "autoupdate", "assets", localRepo)

	if err := u.createTUFRepoDirectory(u.settings.LocalRepoPath, assetPath, AssetDir); err != nil {
		return errors.Wrapf(err, "createTUFRepoDirectory %s", u.settings.LocalRepoPath)
	}
	return nil
}

type assetDirFunc func(string) ([]string, error)

// Creates TUF repo including delegate tree structure on local file system.
// assetDir is the bindata AssetDir function.
func (u *Updater) createTUFRepoDirectory(localPath string, currentAssetPath string, assetDir assetDirFunc) error {
	paths, err := assetDir(currentAssetPath)
	if err != nil {
		return errors.Wrap(err, "assetDir")
	}

	for _, assetPath := range paths {
		fullAssetPath := path.Join(currentAssetPath, assetPath)
		fullLocalPath := filepath.Join(localPath, assetPath)

		// if fullAssetPath is a json file, we should copy it to localPath
		if filepath.Ext(fullAssetPath) == ".json" {
			// The local file should exist and be
			// valid. The starting condition comes from
			// our bundled assets, and it is subsequently
			// updated by TUF. We have seen benign
			// corruption occur, so we want to detect and
			// repair that.
			if ok := u.validLocalFile(fullLocalPath); ok {
				continue
			}

			asset, err := Asset(fullAssetPath)
			if err != nil {
				return errors.Wrap(err, "could not get asset")
			}
			if err := ioutil.WriteFile(fullLocalPath, asset, 0644); err != nil {
				return errors.Wrap(err, "could not write file")
			}
			continue
		}

		// if fullAssetPath is not a JSON file, it's a directory. Create the
		// directory in localPath and recurse into it
		if err := os.MkdirAll(fullLocalPath, 0755); err != nil {
			return errors.Wrapf(err, "mkdir fullLocalPath (%s)", fullLocalPath)
		}
		if err := u.createTUFRepoDirectory(fullLocalPath, fullAssetPath, assetDir); err != nil {
			return errors.Wrap(err, "could not recurse into createTUFRepoDirectory")
		}
	}
	return nil
}

// validLocalFile Checks whether the local file is valid. This was
// originally a simple exists? check, but we've seen this become
// corrupt on disk for benign reasons. So, if it's obviously bad, log
// and allow it to be replaced with the one from assets. (Do not
// attempt to rollback inside the TUF repo, that breaks the
// abstraction of updater)
func (u *Updater) validLocalFile(fullLocalPath string) bool {
	// Check for a missing file. This state is invalid, but we
	// don't need to log about it.
	if _, err := os.Stat(fullLocalPath); os.IsNotExist(err) {
		// No file. While this is invalid, we don't need to log
		return false
	}

	logger := log.With(level.Info(u.logger),
		"msg", "Replacing corrupt TUF file",
		"file", fullLocalPath,
	)

	jsonFile, err := os.Open(fullLocalPath)
	if err != nil {
		logger.Log("err", err)
		return false
	}
	defer jsonFile.Close()

	// Check json validity. We use a Decoder, and not Valid, so we
	// can get the json error back.
	var v interface{}
	if err := json.NewDecoder(jsonFile).Decode(&v); err != nil {
		logger.Log("err", err)
		return false
	}

	return true
}

// UpdaterOption customizes the Updater.
type UpdaterOption func(*Updater)

// WithHTTPClient client configures an http client for the updater.
// If unspecified, http.DefaultClient will be used.
func WithHTTPClient(client *http.Client) UpdaterOption {
	return func(u *Updater) {
		u.client = client
	}
}

// WithSigChannel configures the channel uses for shutdown signaling
func WithSigChannel(sc chan os.Signal) UpdaterOption {
	return func(u *Updater) {
		u.sigChannel = sc
	}
}

// WithUpdate configures the update channel.
// If unspecified, the Updater will use the Stable channel.
func WithUpdateChannel(channel UpdateChannel) UpdaterOption {
	return func(u *Updater) {
		u.updateChannel = channel
	}
}

// WithFinalizer configures an UpdateFinalizer for the updater.
func WithFinalizer(f UpdateFinalizer) UpdaterOption {
	return func(u *Updater) {
		u.finalizer = f
	}
}

// WithMirrorURL configures a MirrorURL in the TUF settings.
func WithMirrorURL(url string) UpdaterOption {
	return func(u *Updater) {
		u.settings.MirrorURL = url
	}
}

// WithLogger configures a logger.
func WithLogger(logger log.Logger) UpdaterOption {
	return func(u *Updater) {
		u.logger = log.With(logger, "caller", log.DefaultCaller)
	}
}

// WithNotaryURL configures a NotaryURL in the TUF settings.
func WithNotaryURL(url string) UpdaterOption {
	return func(u *Updater) {
		u.settings.NotaryURL = url
	}
}

// WithNotaryPrefix configures a prefix for the binaryTargets
func WithNotaryPrefix(prefix string) UpdaterOption {
	return func(u *Updater) {
		u.settings.GUN = path.Join(prefix, u.strippedBinaryName)
	}
}

// override the default bootstrap function for local TUF assets
// only used in tests.
func withoutBootstrap() UpdaterOption {
	return func(u *Updater) {
		u.bootstrapFn = func() error { return nil }
	}
}

// Run starts the updater, which will run until the stop function is called.
func (u *Updater) Run(opts ...tuf.Option) (stop func(), err error) {
	updaterOpts := []tuf.Option{
		tuf.WithHTTPClient(u.client),
		tuf.WithAutoUpdate(u.target, u.stagingPath, u.handler()),
	}
	for _, opt := range opts {
		updaterOpts = append(updaterOpts, opt)
	}

	level.Debug(u.logger).Log(
		"msg", "Running Updater",
		"targetName", u.target,
		"strippedBinaryName", u.strippedBinaryName,
		"LocalRepoPath", u.settings.LocalRepoPath,
		"GUN", u.settings.GUN,
		"stagingPath", u.stagingPath,
		"updatesDirectory", u.updatesDirectory,
	)

	// tuf.NewClient spawns a go thread with a running worker in
	// the background. We don't get much for runtime
	// communication back from it. Some can come in via the
	// UpdateFinalizer function, but it's mostly fire-and-forget
	client, err := tuf.NewClient(
		u.settings,
		updaterOpts...,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "launching %s updater service", filepath.Base(u.binaryName))
	}
	return client.Stop, nil
}

// setTargetPath uses the platform and the binary name to set the
// updater's target to a notary path. Ex: darwin/osquery-stable.tar.gz
func (u *Updater) setTargetPath() error {
	platform, err := osquery.DetectPlatform()
	if err != nil {
		return errors.Wrap(err, "detect platform")
	}

	// filename = <strippedBinaryName>-<update-channel>.tar.gz
	filename := fmt.Sprintf("%s-%s", u.strippedBinaryName, u.updateChannel)
	base := path.Join(string(platform), filename)
	u.target = fmt.Sprintf("%s.tar.gz", base)

	return nil
}

// findCurrentUpdateBinary returns the string path to the current
// downloaded update, to be exec'ed as part of runloop.
func (u *Updater) findCurrentUpdate() string {

	dirEntries, err := ioutil.ReadDir(u.updatesDirectory)
	if err != nil {
		level.Info(u.logger).Log(
			"msg", "Error reading updates directory",
			"updatesDirectory", u.updatesDirectory,
			"err", err,
		)
		return ""
	}

	// iterate backwards over files, looking for a suitable binary
	for i := len(dirEntries) - 1; i >= 0; i-- {
		f := dirEntries[i]

		if !f.IsDir() {
			continue
		}

		potentialBinary := filepath.Join(u.updatesDirectory, f.Name(), u.binaryName)

		stat, err := os.Stat(potentialBinary)
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			level.Info(u.logger).Log(
				"msg", "Error stating potential updated binary",
				"path", potentialBinary,
				"err", err,
			)
			return ""
		case stat.Mode()&0111 == 0:
			level.Info(u.logger).Log(
				"msg", "Potential updated binary is not executable",
				"path", potentialBinary,
			)
			continue
		}

		// Looks good, let's return it!
		return potentialBinary

	}

	level.Debug(u.logger).Log(
		"msg", "No update found",
		"updatesDirectory", u.updatesDirectory,
		"binaryName", u.binaryName,
	)
	return ""
}

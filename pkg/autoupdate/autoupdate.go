// Package autoupdate provides a TUF Updater for the launcher and binaries
// managed by the launcher.
//
// The Updater expects a tar.gz archive with an executable binary, which will replace
// the running binary at the Updater's destination.
package autoupdate

import (
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
	Stable   UpdateChannel = "stable"
	Beta                   = "beta"
	Nightly                = "nightly"
	localDev               = "development"
)

const (
	DefaultMirror = "https://dl.kolide.co"
	DefaultNotary = "https://notary.kolide.co"
)

// Updater is a TUF autoupdater.
type Updater struct {
	settings           *tuf.Settings
	client             *http.Client
	finalizer          func() error
	stagingPath        string
	destination        string
	target             string
	updateChannel      UpdateChannel
	logger             log.Logger
	bootstrapFn        func() error
	strippedBinaryName string
	sigChannel         chan os.Signal
}

// NewUpdater creates a unstarted updater for a specific binary
// updated from a TUF mirror.
func NewUpdater(binaryPath, rootDirectory string, logger log.Logger, opts ...UpdaterOption) (*Updater, error) {
	// There's some chaos between windows and non-windows. In windows,
	// the binaryName ends in .exe, in posix it does not. So, a simple
	// TrimSuffix will handle. *However* this will break if we add the
	// extension. The suffix is inconistent. package-builder has a lot
	// of gnarly code around that. We may need to import it.
	binaryName := filepath.Base(binaryPath)
	strippedBinaryName := strings.TrimSuffix(binaryName, ".exe")
	tufRepoPath := filepath.Join(rootDirectory, fmt.Sprintf("%s-tuf", strippedBinaryName))
	stagingPath := filepath.Join(filepath.Dir(binaryPath), fmt.Sprintf("%s-staging", binaryName))
	gun := fmt.Sprintf("kolide/%s", strippedBinaryName)

	settings := tuf.Settings{
		LocalRepoPath: tufRepoPath,
		NotaryURL:     DefaultNotary,
		GUN:           gun,
		MirrorURL:     DefaultMirror,
	}

	updater := Updater{
		settings:           &settings,
		destination:        binaryPath,
		stagingPath:        stagingPath,
		updateChannel:      Stable,
		client:             http.DefaultClient,
		logger:             logger,
		finalizer:          func() error { return nil },
		strippedBinaryName: strippedBinaryName,
	}

	// create TUF from local assets, but allow overriding with a no-op in tests.
	updater.bootstrapFn = updater.createLocalTufRepo

	for _, opt := range opts {
		opt(&updater)
	}

	var err error
	updater.target, err = updater.setTargetPath()
	if err != nil {
		return nil, errors.Wrapf(err, "set updater target for destination %s", binaryPath)
	}
	if err := updater.bootstrapFn(); err != nil {
		return nil, errors.Wrap(err, "creating local TUF repo")
	}

	return &updater, nil
}

// createLocalTufRepo bootstraps local TUF metadata from bindata
// assets. (TUF requires an initial starting repo)
func (u *Updater) createLocalTufRepo() error {
	if err := os.MkdirAll(u.settings.LocalRepoPath, 0755); err != nil {
		return err
	}
	localRepo := filepath.Base(u.settings.LocalRepoPath)
	assetPath := path.Join("pkg", "autoupdate", "assets", localRepo)

	if err := u.createTUFRepoDirectory(u.settings.LocalRepoPath, assetPath, AssetDir); err != nil {
		return err
	}
	return nil
}

type assetDirFunc func(string) ([]string, error)

// Creates TUF repo including delegate tree structure on local file system.
// assetDir is the bindata AssetDir function.
func (u *Updater) createTUFRepoDirectory(localPath string, currentAssetPath string, assetDir assetDirFunc) error {
	paths, err := assetDir(currentAssetPath)
	if err != nil {
		return err
	}

	for _, assetPath := range paths {
		fullAssetPath := path.Join(currentAssetPath, assetPath)
		fullLocalPath := filepath.Join(localPath, assetPath)

		// if fullAssetPath is a json file, we should copy it to localPath
		if filepath.Ext(fullAssetPath) == ".json" {
			// We need to ensure the file exists, but if it exists it has
			// additional state. So, create when not present. This helps
			// with an issue where the directory would be created, but the
			// files not yet yet there -- Generating an invalid state. Note:
			// this does not check the validity of the files, they might be
			// corrupt.
			if _, err := os.Stat(fullLocalPath); !os.IsNotExist(err) {
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
			return err
		}
		if err := u.createTUFRepoDirectory(fullLocalPath, fullAssetPath, assetDir); err != nil {
			return errors.Wrap(err, "could not recurse into createTUFRepoDirectory")
		}
	}
	return nil
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
		u.logger = logger
	}
}

// WithNotaryURL configures a NotaryURL in the TUF settings.
func WithNotaryURL(url string) UpdaterOption {
	return func(u *Updater) {
		u.settings.NotaryURL = url
	}
}

// override the default bootstrap function for local TUF assets
// only used in tests.
func withoutBootstrap() UpdaterOption {
	return func(u *Updater) {
		u.bootstrapFn = func() error { return nil }
	}
}

// UpdateFinalizer is executed after the Updater updates a destination.
// The UpdateFinalizer is usually a function which will handle restarting the updated binary.
type UpdateFinalizer func() error

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
	)

	client, err := tuf.NewClient(
		u.settings,
		updaterOpts...,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "launching %s updater service", filepath.Base(u.destination))
	}
	return client.Stop, nil
}

// target creates a TUF target for a binary using the Destination.
// Ex: darwin/osquery-stable.tar.gz
func (u *Updater) setTargetPath() (string, error) {
	platform, err := osquery.DetectPlatform()
	if err != nil {
		return "", err
	}

	// filename = <strippedBinaryName>-<update-channel>.tar.gz
	filename := fmt.Sprintf("%s-%s", u.strippedBinaryName, u.updateChannel)
	base := path.Join(string(platform), filename)
	return fmt.Sprintf("%s.tar.gz", base), nil
}

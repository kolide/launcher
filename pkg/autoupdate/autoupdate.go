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

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
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
	settings      *tuf.Settings
	client        *http.Client
	finalizer     func() error
	stagingPath   string
	destination   string
	target        string
	updateChannel UpdateChannel
	logger        log.Logger
	bootstrapFn   func() error
}

// NewUpdater creates a unstarted updater for a specific binary
// updated from a TUF mirror.
func NewUpdater(binaryPath, rootDirectory string, logger log.Logger, opts ...UpdaterOption) (*Updater, error) {
	binaryName := filepath.Base(binaryPath)
	tufRepoPath := filepath.Join(rootDirectory, fmt.Sprintf("%s-tuf", binaryName))
	stagingPath := filepath.Join(filepath.Dir(binaryPath), fmt.Sprintf("%s-staging", binaryName))
	gun := fmt.Sprintf("kolide/%s", binaryName)

	settings := tuf.Settings{
		LocalRepoPath: tufRepoPath,
		NotaryURL:     DefaultNotary,
		GUN:           gun,
		MirrorURL:     DefaultMirror,
	}

	updater := Updater{
		settings:      &settings,
		destination:   binaryPath,
		stagingPath:   stagingPath,
		updateChannel: Stable,
		client:        http.DefaultClient,
		logger:        logger,
		finalizer:     func() error { return nil },
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

// bootstraps local TUF metadata from bindata assets.
func (u *Updater) createLocalTufRepo() error {
	// We don't want to overwrite an existing repo as it stores state between installations
	if _, err := os.Stat(u.settings.LocalRepoPath); !os.IsNotExist(err) {
		level.Debug(u.logger).Log("msg", "not creating new TUF repositories because they already exist")
		return nil
	}

	if err := os.MkdirAll(u.settings.LocalRepoPath, 0755); err != nil {
		return err
	}
	localRepo := filepath.Base(u.settings.LocalRepoPath)
	assetPath := path.Join("pkg", "autoupdate", "assets", localRepo)
	if err := createTUFRepoDirectory(u.settings.LocalRepoPath, assetPath, AssetDir); err != nil {
		return err
	}
	return nil
}

type assetDirFunc func(string) ([]string, error)

// Creates TUF repo including delegate tree structure on local file system.
// assetDir is the bindata AssetDir function.
func createTUFRepoDirectory(localPath string, currentAssetPath string, assetDir assetDirFunc) error {
	paths, err := assetDir(currentAssetPath)
	if err != nil {
		return err
	}

	for _, assetPath := range paths {
		fullAssetPath := path.Join(currentAssetPath, assetPath)
		fullLocalPath := filepath.Join(localPath, assetPath)

		// if fullAssetPath is a json file, we should copy it to localPath
		if filepath.Ext(fullAssetPath) == ".json" {
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
		if err := createTUFRepoDirectory(fullLocalPath, fullAssetPath, assetDir); err != nil {
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
	client, err := tuf.NewClient(
		u.settings,
		updaterOpts...,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "launching %s updater service", filepath.Base(u.destination))
	}
	return client.Stop, nil
}

// The handler is called by the tuf package when tuf detects a change with
// the remote metadata.
// The handler method will do the following:
// 1) untar the staged staged file,
// 2) replace the existing binary,
// 3) call the Updater's finalizer method, usually a restart function for the running binary.
func (u *Updater) handler() tuf.NotificationHandler {
	return func(stagingPath string, err error) {
		u.logger.Log("msg", "new staged tuf file", "file", stagingPath, "target", u.target, "binary", u.destination)

		if err != nil {
			u.logger.Log("msg", "download failed", "target", u.target, "err", err)
			return
		}

		if err := fs.UntarBundle(stagingPath, stagingPath); err != nil {
			u.logger.Log("msg", "untar downloaded target", "binary", u.target, "err", err)
			return
		}

		binary := filepath.Join(filepath.Dir(stagingPath), filepath.Base(u.destination))
		if err := os.Rename(binary, u.destination); err != nil {
			u.logger.Log("msg", "update binary from staging dir", "binary", u.destination, "err", err)
			return
		}

		if err := os.Chmod(u.destination, 0755); err != nil {
			u.logger.Log("msg", "setting +x permissions on binary", "binary", u.destination, "err", err)
			return
		}

		if err := u.finalizer(); err != nil {
			u.logger.Log("msg", "calling restart function for updated binary", "binary", u.destination, "err", err)
			return
		}

		u.logger.Log("msg", "completed update for binary", "binary", u.destination)
	}
}

// target creates a TUF target for a binary using the Destination.
// Ex: darwin/osquery-stable.tar.gz
func (u *Updater) setTargetPath() (string, error) {
	platform, err := osquery.DetectPlatform()
	if err != nil {
		return "", err
	}

	// filename = <binary>-<update-channel>.tar.gz
	filename := fmt.Sprintf("%s-%s", filepath.Base(u.destination), u.updateChannel)
	base := path.Join(string(platform), filename)
	return fmt.Sprintf("%s.tar.gz", base), nil
}

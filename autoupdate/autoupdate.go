// Package autoupdate provides a TUF Updater for the launcher and binaries
// managed by the launcher.
//
// The Updater expects a tar.gz archive with an executable binary, which will replace
// the running binary at the Updater's destination.
package autoupdate

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/osquery"
	"github.com/kolide/updater/tuf"
	"github.com/pkg/errors"
)

const (
	defaultMirror = "https://dl.kolide.com"
	defaultNotary = "https://notary.kolide.com"
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
func NewUpdater(
	d Destination,
	metadataRoot string,
	opts ...UpdaterOption,
) (*Updater, error) {
	settings := tuf.Settings{
		LocalRepoPath: d.tufRepoPath(metadataRoot),
		NotaryURL:     defaultNotary,
		GUN:           d.gun(),
		MirrorURL:     defaultMirror,
	}

	updater := Updater{
		settings:      &settings,
		destination:   string(d),
		stagingPath:   d.stagingPath(),
		updateChannel: Stable,
		client:        http.DefaultClient,
		logger:        log.NewNopLogger(),
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
		return nil, errors.Wrapf(err, "set updater target for destination %s", d)
	}
	if err := updater.bootstrapFn(); err != nil {
		return nil, errors.Wrap(err, "creating local TUF repo")
	}

	return &updater, nil
}

// bootstraps local TUF metadata from bindata assets.
func (u *Updater) createLocalTufRepo() error {
	if err := os.MkdirAll(u.settings.LocalRepoPath, 0755); err != nil {
		return err
	}

	localRepo := filepath.Base(u.settings.LocalRepoPath)
	roles := []string{"root.json", "snapshot.json", "timestamp.json", "targets.json"}
	for _, role := range roles {
		asset, err := Asset(path.Join("autoupdate", "assets", localRepo, role))
		if err != nil {
			return err
		}
		localPath := filepath.Join(u.settings.LocalRepoPath, role)
		if err := ioutil.WriteFile(localPath, asset, 0644); err != nil {
			return err
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

// UpdateFinalizer is executed after the Updater updates a Destination.
// The UpdateFinalizer is usually a function which will handle restarting the updated binary.
type UpdateFinalizer func() error

// Destination is a binary path which will be replaced by the Updater.
type Destination string

// Before replacing the running binary, the updater must download and untar it from the remote server.
// The running binary is replaced only if the download is successful.
func (d Destination) stagingPath() string {
	bin := string(d)
	return filepath.Join(
		filepath.Dir(bin),
		fmt.Sprintf("%s-staging", filepath.Base(bin)),
	)
}

func (d Destination) tufRepoPath(root string) string {
	return filepath.Join(
		root,
		fmt.Sprintf("%s-tuf", filepath.Base(string(d))),
	)
}

// The TUF GUN is kolide/<binary_name>
func (d Destination) gun() string {
	bin := filepath.Base(string(d))
	return fmt.Sprintf("kolide/%s", bin)
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

		if err := untarDownload(stagingPath, stagingPath); err != nil {
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

func untarDownload(destination string, source string) error {
	f, err := os.Open(source)
	if err != nil {
		return errors.Wrap(err, "autoupdate: open download source")
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return errors.Wrapf(err, "autoupdate: create gzip reader from %s", source)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "autoupdate: reading tar file")
		}

		path := filepath.Join(filepath.Dir(destination), header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return errors.Wrapf(err, "autoupdate: creating directory for tar file: %s", path)
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return errors.Wrapf(err, "autoupdate: open file %s", path)
		}
		defer file.Close()
		if _, err := io.Copy(file, tr); err != nil {
			return errors.Wrapf(err, "autoupdate: copy tar %s to destination %s", header.FileInfo().Name(), path)
		}
	}
	return nil
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

// UpdateChannel determines the TUF target for a Updater.
// The Default UpdateChannel is Stable.
type UpdateChannel string

const (
	Stable   UpdateChannel = "stable"
	Beta                   = "beta"
	Nightly                = "nightly"
	localDev               = "development"
)

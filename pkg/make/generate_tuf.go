package make

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"github.com/theupdateframework/notary/client"
	"github.com/theupdateframework/notary/trustpinning"
	"github.com/theupdateframework/notary/tuf/data"
	"go.opencensus.io/trace"
)

type notaryRemoteServer struct {
	URL string `json:"url"`
}

type notaryConf struct {
	RemoteServer notaryRemoteServer `json:"remote_server"`
}

// GenerateTUF setups up a bunch of TUF metadata for the auto
// updater. There are several steps:
//
// 1. Generate a bindata file from an empty directory so that the symbol are present (Asset, AssetDir, etc)
// 2. Run generate_tuf.go tool to generate actual TUF metadata
// 3. Recreate the bindata file with the real TUF metadata
func (b *Builder) GenerateTUF(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "make.GenerateTUF")
	defer span.End()

	dir, err := ioutil.TempDir("", "bootstrap-launcher-autoupdate-bindata")
	if err != nil {
		return errors.Wrapf(err, "create empty dir for bindata")
	}
	// defer os.RemoveAll(dir)
	fmt.Printf("using %s as a dir\n", dir)

	// asset bundle on an empty directory. Not sure if this is still
	// needed, but it's always been this way.
	if err := b.execBindata(ctx, dir); err != nil {
		return errors.Wrap(err, "exec bindata for empty dir")
	}

	//
	// Populate the asset dir, and package it.
	//

	// config file pulls in values from the autoupdate package
	notaryConfig := notaryConf{
		RemoteServer: notaryRemoteServer{URL: autoupdate.DefaultNotary},
	}

	configFileOut, err := os.Create(filepath.Join(dir, "config.json"))
	if err != nil {
		return errors.Wrap(err, "opening config.json")
	}
	defer configFileOut.Close()

	jsonEnc := json.NewEncoder(configFileOut)
	if err = jsonEnc.Encode(notaryConfig); err != nil {
		return errors.Wrap(err, "writing config.json")
	}

	// binaries that are autoupdated.
	binaryTargets := []string{
		"osqueryd",
		"launcher",
	}

	for _, t := range binaryTargets {
		level.Debug(ctxlog.FromContext(ctx)).Log("target", "generate-tuf", "msg", "bootstrap notary", "binary", t, "remote_server_url", notaryConfig.RemoteServer.URL)
		gun := path.Join(autoupdate.DefaultNotaryPrefix, t)
		localRepo := filepath.Join("pkg", "autoupdate", "assets", fmt.Sprintf("%s-tuf", t))
		if err := os.MkdirAll(localRepo, 0755); err != nil {
			return errors.Wrapf(err, "make autoupdate dir %s", localRepo)
		}

		if err := bootstrapFromNotary(dir, notaryConfig.RemoteServer.URL, localRepo, gun); err != nil {
			return errors.Wrapf(err, "bootstrap notary GUN %s", gun)
		}
	}

	if err := b.execBindata(ctx, "pkg/autoupdate/assets/..."); err != nil {
		return errors.Wrap(err, "exec bindata for autoupdate assets")
	}

	return nil
}

func bootstrapFromNotary(notaryConfigDir, remoteServerURL, localRepo, gun string) error {
	passwordRetrieverFn := func(key, alias string, createNew bool, attempts int) (pass string, giveUp bool, err error) {
		pass = os.Getenv(key)
		if pass == "" {
			err = fmt.Errorf("Missing pass phrase env var %q", key)
		}
		return pass, giveUp, err
	}

	// Safely fetch and validate all TUF metadata from remote Notary server.
	repo, err := client.NewFileCachedRepository(
		notaryConfigDir,
		data.GUN(gun),
		remoteServerURL,
		&http.Transport{Proxy: http.ProxyFromEnvironment},
		passwordRetrieverFn,
		trustpinning.TrustPinConfig{},
	)
	if err != nil {
		return errors.Wrap(err, "create an instance of the TUF repository")
	}

	if _, err := repo.GetAllTargetMetadataByName(""); err != nil {
		return errors.Wrap(err, "getting all target metadata")
	}

	// Stage TUF metadata and create bindata from it so it can be distributed as part of the Launcher executable
	source := filepath.Join(notaryConfigDir, "tuf", gun, "metadata")
	if err := fs.CopyDir(source, localRepo); err != nil {
		return errors.Wrap(err, "copying TUF repo metadata")
	}

	return nil
}

// execBindata runs go-bindat as asset packaging.
func (b *Builder) execBindata(ctx context.Context, dir string) error {
	ctx, span := trace.StartSpan(ctx, "make.execBindata")
	defer span.End()

	cmd := b.execCC(
		ctx,
		"go-bindata",
		"-o", "pkg/autoupdate/bindata.go",
		"-pkg", "autoupdate",
		dir,
	)
	// 	cmd.Env = append(cmd.Env, b.cmdEnv...)
	out, err := cmd.CombinedOutput()
	return errors.Wrapf(err, "run bindata for dir %s, output=%s", dir, out)
}

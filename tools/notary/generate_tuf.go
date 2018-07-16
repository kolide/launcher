package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/pkg/errors"
	"github.com/theupdateframework/notary/client"
	"github.com/theupdateframework/notary/trustpinning"
	"github.com/theupdateframework/notary/tuf/data"
)

func main() {
	var (
		flBinary          = flag.String("binary", "osqueryd", "which binary to use for assets")
		flNotaryConfigDir = flag.String("notary_config_dir", filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/tools/notary/config"), "notary base directory")
	)
	flag.Parse()

	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	gun := path.Join("kolide", *flBinary)
	localRepo := filepath.Join("pkg", "autoupdate", "assets", fmt.Sprintf("%s-tuf", *flBinary))

	if err := os.MkdirAll(localRepo, 0755); err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	if err := bootstrapFromNotary(*flNotaryConfigDir, localRepo, gun); err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	level.Info(logger).Log(
		"msg", "successfully bootstrapped and validated TUF repo",
		"gun", gun,
	)
}

func bootstrapFromNotary(notaryConfigDir, localRepo, gun string) error {
	// Read Notary configuration
	notaryConfigFile, err := os.Open(filepath.Join(notaryConfigDir, "config.json"))
	if err != nil {
		return errors.Wrap(err, "opening notary config file")
	}
	defer notaryConfigFile.Close()

	// Decode the Notary configuration into a struct
	conf := notaryConfig{}
	if err = json.NewDecoder(notaryConfigFile).Decode(&conf); err != nil {
		return errors.Wrap(err, "decoding notary config file")
	}

	// Safely fetch and validate all TUF metadata from remote Notary server.
	repo, err := client.NewFileCachedRepository(
		notaryConfigDir,
		data.GUN(gun),
		conf.RemoteServer.URL,
		&http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		passwordRetriever,
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

type notaryConfig struct {
	RemoteServer struct {
		URL string `json:"url"`
	} `json:"remote_server"`
}

func passwordRetriever(key, alias string, createNew bool, attempts int) (pass string, giveUp bool, err error) {
	pass = os.Getenv(key)
	if pass == "" {
		err = fmt.Errorf("Missing pass phrase env var %q", key)
	}
	return pass, giveUp, err
}

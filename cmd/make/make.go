package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/logutil"
	"github.com/pkg/errors"
	"github.com/theupdateframework/notary/client"
	"github.com/theupdateframework/notary/trustpinning"
	"github.com/theupdateframework/notary/tuf/data"
	"golang.org/x/sync/errgroup"
)

func main() {
	buildAll := strings.Join([]string{
		"deps-go",
		"install-tools",
	}, ",")

	var (
		flTargets = flag.String("targets", buildAll, "comma separated list of targets")
		flDebug   = flag.Bool("debug", false, "use a debug logger")
	)
	flag.Parse()

	logger := logutil.NewCLILogger(*flDebug)
	b := builder{logger: logger}
	targetSet := map[string]func() error{
		"deps-go":       b.depsGo,
		"install-tools": b.installTools,
		"generate-tuf":  b.generateTUF,
	}

	if t := strings.Split(*flTargets, ","); len(t) != 0 && t[0] != "" {
		for _, target := range t {
			if fn, ok := targetSet[target]; ok {
				level.Debug(logger).Log("msg", "calling target", "target", target)
				fn()
			} else {
				logutil.Fatal(logger, "err", "target does not exist", "target", target)
			}
		}
	}
}

type builder struct {
	logger log.Logger
}

func (b *builder) goVersionCompatible() error {
	goConstraint := ">= 1.11"
	c, _ := semver.NewConstraint(goConstraint)
	gov := strings.TrimPrefix(runtime.Version(), "go")
	v, err := semver.NewVersion(gov)
	if err != nil {
		return errors.Wrapf(err, "parse go version %q as semver", gov)
	}
	if !c.Check(v) {
		return errors.Errorf("project requires Go version %s, have %s", goConstraint, gov)
	}
	return nil
}

func (b *builder) depsGo() error {
	if err := b.goVersionCompatible(); err != nil {
		return err
	}
	out, err := exec.Command("go", "mod", "download").CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "run go mod download, output=%s", out)
	}
	level.Debug(b.logger).Log("cmd", "go mod download", "output", string(out))
	return nil
}

func (b *builder) installTools() error {
	cmd := exec.Command(
		"go", "list",
		"-tags", "tools",
		"-json",
		"github.com/kolide/launcher/pkg/tools",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "create stdout pipe for go list command")
	}
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "run go list command, %s", stderr)
	}

	var list struct {
		Imports []string
	}
	if err := json.NewDecoder(stdout).Decode(&list); err != nil {
		return errors.Wrap(err, "decode go list output")
	}

	var g errgroup.Group
	for _, toolPath := range list.Imports {
		toolPath := toolPath
		_, tool := path.Split(toolPath)
		path, err := exec.LookPath(tool)
		if err == nil {
			level.Debug(b.logger).Log(
				"target", "install tools",
				"tool", tool,
				"exists", true,
				"path", path,
			)
			continue
		}
		g.Go(func() error {
			return b.installTool(toolPath)
		})
	}
	err = g.Wait()
	return errors.Wrap(err, "install tools")
}

func (b *builder) installTool(importPath string) error {
	out, err := exec.Command("go", "install", importPath).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "run go install %s, output=%s", importPath, out)
	}
	level.Debug(b.logger).Log("target", "install tool", "import_path", importPath, "output", string(out))
	return nil
}

func (b *builder) generateTUF() error {
	/* First, we generate a bindata file from an empty directory so that the symbols
	   are present (Asset, AssetDir, etc). Once the symbols are present, we can run
	   the generate_tuf.go tool to generate actual TUF metadata. Finally, we recreate
	   the bindata file with the real TUF metadata.
	*/
	dir, err := ioutil.TempDir("", "bootstrap-launcher-bindata")
	if err != nil {
		return errors.Wrapf(err, "create empty dir for bindata")
	}
	defer os.RemoveAll(dir)

	if err := execBindata(dir); err != nil {
		return errors.Wrap(err, "exec bindata for empty dir")
	}

	binaryTargets := []string{ // binaries that are autoupdated.
		"osqueryd",
		"launcher",
	}

	notaryConfigDir := filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/tools/notary/config")
	notaryConfigFile, err := os.Open(filepath.Join(notaryConfigDir, "config.json"))
	if err != nil {
		return errors.Wrap(err, "opening notary config file")
	}
	defer notaryConfigFile.Close()
	var conf struct {
		RemoteServer struct {
			URL string `json:"url"`
		} `json:"remote_server"`
	}
	if err = json.NewDecoder(notaryConfigFile).Decode(&conf); err != nil {
		return errors.Wrap(err, "decoding notary config file")
	}

	for _, t := range binaryTargets {
		level.Debug(b.logger).Log("target", "generate-tuf", "msg", "bootstrap notary", "binary", t, "remote_server_url", conf.RemoteServer.URL)
		gun := path.Join("kolide", t)
		localRepo := filepath.Join("pkg", "autoupdate", "assets", fmt.Sprintf("%s-tuf", t))
		if err := os.MkdirAll(localRepo, 0755); err != nil {
			return errors.Wrapf(err, "make autoupdate dir %s", localRepo)
		}

		if err := bootstrapFromNotary(notaryConfigDir, conf.RemoteServer.URL, localRepo, gun); err != nil {
			return errors.Wrapf(err, "bootstrap notary GUN %s", gun)
		}
	}

	if err := execBindata("pkg/autoupdate/assets/..."); err != nil {
		return errors.Wrap(err, "exec bindata for autoupdate assets")
	}

	return nil
}

func execBindata(dir string) error {
	out, err := exec.Command(
		"go-bindata",
		"-o", "pkg/autoupdate/bindata.go",
		"-pkg", "autoupdate",
		dir,
	).CombinedOutput()
	return errors.Wrapf(err, "run bindata for dir %s, output=%s", dir, out)
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

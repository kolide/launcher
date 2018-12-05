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
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
		flTargets      = flag.String("targets", buildAll, "comma separated list of targets")
		flDebug        = flag.Bool("debug", false, "use a debug logger")
		flBuildARCH    = flag.String("arch", runtime.GOARCH, "Architecture to build for.")
		flBuildOS      = flag.String("os", runtime.GOOS, "Operating system to build for.")
		flRace         = flag.Bool("race", false, "Build race-detector version of binaries.")
		flStatic       = flag.Bool("static", false, "Build a static binary.")
		flStampVersion = flag.Bool("linkstamp", false, "Add version info with ldflags.")
	)
	flag.Parse()

	logger := logutil.NewCLILogger(*flDebug)
	b := builder{
		logger:       logger,
		os:           *flBuildOS,
		arch:         *flBuildARCH,
		race:         *flRace,
		static:       *flStatic,
		stampVersion: *flStampVersion,
	}
	targetSet := map[string]func() error{
		"deps-go":         b.depsGo,
		"install-tools":   b.installTools,
		"generate-tuf":    b.generateTUF,
		"launcher":        b.buildCmd("./cmd/launcher", b.platformBinary("launcher")),
		"extension":       b.buildCmd("./cmd/osquery-extension", b.extBinary("osquery-extension")),
		"table-extension": b.buildCmd("./cmd/launcher.ext", b.extBinary("tables")),
		"grpc-extension":  b.buildCmd("./cmd/grpc.ext", b.extBinary("grpc")),
		"package-builder": b.buildCmd("./cmd/package-builder", b.platformBinary("package-builder")),
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
	logger       log.Logger
	os           string
	arch         string
	static       bool
	race         bool
	stampVersion bool
}

func (b *builder) extBinary(input string) string {
	input = filepath.Join("build", b.os, input)
	if b.os == "windows" {
		return input + ".exe"
	} else {
		return input + ".ext"
	}
}

func (b *builder) platformBinary(input string) string {
	input = filepath.Join("build", b.os, input)
	if b.os == "windows" {
		return input + ".exe"
	}
	return input
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

func (b *builder) buildCmd(src, output string) func() error {
	return func() error {
		_, appName := filepath.Split(output)
		baseArgs := []string{"build", "-o", output}
		if b.race {
			baseArgs = append(baseArgs, "-race")
		}
		var ldFlags []string
		if b.static {
			ldFlags = append(ldFlags, "-w -d -linkmode internal")
		}
		if b.stampVersion {
			v, err := execOut("git", "describe", "--tags", "--always", "--dirty")
			if err != nil {
				return err
			}

			branch, err := execOut("git", "rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return err
			}

			revision, err := execOut("git", "rev-parse", "HEAD")
			if err != nil {
				return err
			}

			usr, err := user.Current()
			if err != nil {
				return err
			}

			ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/kit/version.appName=%s"`, appName))
			ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/kit/version.version=%s"`, v))
			ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/kit/version.branch=%s"`, branch))
			ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/kit/version.revision=%s"`, revision))
			ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/kit/version.buildDate=%s"`, time.Now().UTC().Format("2006-01-02")))
			ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/kit/version.buildUser=%s (%s)"`, usr.Name, usr.Username))
			ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/kit/version.goVersion=%s"`, runtime.Version()))
		}
		if len(ldFlags) != 0 {
			baseArgs = append(baseArgs, fmt.Sprintf("--ldflags=%s", strings.Join(ldFlags, " ")))
		}
		args := append(baseArgs, src)

		level.Debug(b.logger).Log("mgs", "building binary", "app_name", appName, "output", output, "go_args", strings.Join(args, "  "))

		cmd := exec.Command("go", args...)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "GO111MODULE=on")
		cmd.Env = append(cmd.Env, fmt.Sprintf("GOOS=%s", b.os))
		cmd.Env = append(cmd.Env, fmt.Sprintf("GOARCH=%s", b.arch))
		if b.static {
			cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}

		// all the builds go to `build/<os>/binary`, but if the build OS is the same as the target OS,
		// we also want to hardlink the resulting binary at the root of `build/` for convenience.
		// ex: running ./build/launcher on macos instead of ./build/darwin/launcher
		if b.os == runtime.GOOS {
			platformPath := filepath.Join("build", appName)
			if err := os.Remove(platformPath); err != nil {
				// log but don't fail. This could happen if for example ./build/launcher.exe is referenced by a running service.
				// if this becomes clearer, we can either return an error here, or go back to silently ignoring.
				level.Debug(b.logger).Log("msg", "remove before hardlink failed", "err", err, "app_name", appName)
			}
			return os.Link(output, platformPath)
		}
		return nil
	}
}

func execOut(argv0 string, args ...string) (string, error) {
	cmd := exec.Command(argv0, args...)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "run command %s %v, stderr=%s", argv0, args, stderr)
	}
	return strings.TrimSpace(stdout.String()), nil
}

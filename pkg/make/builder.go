/* Package make provides some simple functions to handle build and go
dependancies.

We used to do this with gnumake rules, but as we added windows
compatibility, we found make too limiting. Moving this into go allows
us to write cleaner cross-platform code.
*/

package make

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"github.com/theupdateframework/notary/client"
	"github.com/theupdateframework/notary/trustpinning"
	"github.com/theupdateframework/notary/tuf/data"
	"go.opencensus.io/trace"
	"golang.org/x/sync/errgroup"
)

type Builder struct {
	os           string
	arch         string
	static       bool
	race         bool
	stampVersion bool

	goVer  *semver.Version
	cmdEnv []string
}

type Option func(*Builder)

func WithOs(o string) Option {
	return func(b *Builder) {
		b.os = o
	}
}

func WithArch(a string) Option {
	return func(b *Builder) {
		b.arch = a
	}
}

func WithStatic() Option {
	return func(b *Builder) {
		b.static = true
	}
}

func WithRace() Option {
	return func(b *Builder) {
		b.race = true
	}
}

func WithStampVersion() Option {
	return func(b *Builder) {
		b.stampVersion = true
	}
}

// execCommand is a var to allow mocking in tesst
var execCommand = exec.Command

// execCommandContext is a var to allow mocking in tests
var execCommandContext = exec.CommandContext

func New(opts ...Option) (*Builder, error) {

	verString := strings.TrimPrefix(runtime.Version(), "go")
	goVer, err := semver.NewVersion(verString)
	if err != nil {
		return nil, errors.Wrapf(err, "parse go version %q as semver", verString)
	}

	b := Builder{
		os:    runtime.GOOS,
		arch:  runtime.GOARCH,
		goVer: goVer,
	}

	for _, opt := range opts {
		opt(&b)
	}

	// Some default environment things
	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, "GO111MODULE=on")
	cmdEnv = append(cmdEnv, fmt.Sprintf("GOOS=%s", b.os))
	cmdEnv = append(cmdEnv, fmt.Sprintf("GOARCH=%s", b.arch))
	if b.static {
		cmdEnv = append(cmdEnv, "CGO_ENABLED=0")
	}

	b.cmdEnv = cmdEnv

	return &b, nil

}

// ExtBinary is a helper to return the platform specific extension name.
func (b *Builder) ExtBinary(input string) string {
	input = filepath.Join("build", b.os, input)
	if b.os == "windows" {
		return input + ".exe"
	} else {
		return input + ".ext"
	}
}

// PlatformBinary is a helper to return the platform specific binary suffix.
func (b *Builder) PlatformBinary(input string) string {
	input = filepath.Join("build", b.os, input)
	if b.os == "windows" {
		return input + ".exe"
	}
	return input
}

func (b *Builder) goVersionCompatible() error {
	if b.goVer == nil {
		return errors.New("no go version. Is this a bad mock?")
	}
	goConstraint := ">= 1.11"
	c, _ := semver.NewConstraint(goConstraint)
	if !c.Check(b.goVer) {
		return errors.Errorf("project requires Go version %s, have %s", goConstraint, b.goVer)
	}
	return nil
}

func (b *Builder) DepsGo(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "make.DepsGo")
	defer span.End()

	if err := b.goVersionCompatible(); err != nil {
		return err
	}
	cmd := execCommandContext(ctx, "go", "mod", "download")
	cmd.Env = append(cmd.Env, b.cmdEnv...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "run go mod download, output=%s", out)
	}
	level.Debug(ctxlog.FromContext(ctx)).Log("cmd", "go mod download", "output", string(out))
	return nil
}

func (b *Builder) InstallTools(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "make.InstallTools")
	defer span.End()

	cmd := execCommandContext(
		ctx,
		"go", "list",
		"-tags", "tools",
		"-json",
		"github.com/kolide/launcher/pkg/tools",
	)
	cmd.Env = append(cmd.Env, b.cmdEnv...)
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
			level.Debug(ctxlog.FromContext(ctx)).Log(
				"target", "install tools",
				"tool", tool,
				"exists", true,
				"path", path,
			)
			continue
		}
		// TODO for go 1.12, this can be parallized.
		g.Go(func() error {
			return b.installTool(ctx, toolPath)
		})
	}
	err = g.Wait()
	return errors.Wrap(err, "install tools")
}

func (b *Builder) installTool(ctx context.Context, importPath string) error {
	ctx, span := trace.StartSpan(ctx, "make.installTool")
	defer span.End()

	cmd := execCommandContext(ctx, "go", "install", importPath)
	cmd.Env = append(cmd.Env, b.cmdEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "run go install %s, output=%s", importPath, out)
	}
	level.Debug(ctxlog.FromContext(ctx)).Log("target", "install tool", "import_path", importPath, "output", string(out))
	return nil
}

func (b *Builder) GenerateTUF(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "make.GenerateTUF")
	defer span.End()

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

	if err := execBindata(ctx, dir); err != nil {
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
		level.Debug(ctxlog.FromContext(ctx)).Log("target", "generate-tuf", "msg", "bootstrap notary", "binary", t, "remote_server_url", conf.RemoteServer.URL)
		gun := path.Join("kolide", t)
		localRepo := filepath.Join("pkg", "autoupdate", "assets", fmt.Sprintf("%s-tuf", t))
		if err := os.MkdirAll(localRepo, 0755); err != nil {
			return errors.Wrapf(err, "make autoupdate dir %s", localRepo)
		}

		if err := bootstrapFromNotary(notaryConfigDir, conf.RemoteServer.URL, localRepo, gun); err != nil {
			return errors.Wrapf(err, "bootstrap notary GUN %s", gun)
		}
	}

	if err := execBindata(ctx, "pkg/autoupdate/assets/..."); err != nil {
		return errors.Wrap(err, "exec bindata for autoupdate assets")
	}

	return nil
}

func execBindata(ctx context.Context, dir string) error {
	ctx, span := trace.StartSpan(ctx, "make.execBindata")
	defer span.End()

	cmd := execCommandContext(
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

func (b *Builder) BuildCmd(src, output string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, appName := filepath.Split(output)

		ctx, span := trace.StartSpan(ctx, fmt.Sprintf("make.BuildCmd.%s", appName))
		defer span.End()

		baseArgs := []string{"build", "-o", output}
		if b.race {
			baseArgs = append(baseArgs, "-race")
		}
		var ldFlags []string
		if b.static {
			ldFlags = append(ldFlags, "-w -d -linkmode internal")
		}
		if b.stampVersion {
			v, err := execOut(ctx, "git", "describe", "--tags", "--always", "--dirty")
			if err != nil {
				return err
			}

			branch, err := execOut(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return err
			}

			revision, err := execOut(ctx, "git", "rev-parse", "HEAD")
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

		level.Debug(ctxlog.FromContext(ctx)).Log("mgs", "building binary", "app_name", appName, "output", output, "go_args", strings.Join(args, "  "))

		cmd := execCommandContext(ctx, "go", args...)
		cmd.Env = append(cmd.Env, b.cmdEnv...)
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
			if err := os.Remove(platformPath); err != nil && !os.IsNotExist(err) {
				// log but don't fail. This could happen if for example ./build/launcher.exe is referenced by a running service.
				// if this becomes clearer, we can either return an error here, or go back to silently ignoring.
				level.Debug(ctxlog.FromContext(ctx)).Log("msg", "remove before hardlink failed", "err", err, "app_name", appName)
			}
			return os.Link(output, platformPath)
		}
		return nil
	}
}

func execOut(ctx context.Context, argv0 string, args ...string) (string, error) {
	cmd := execCommandContext(ctx, argv0, args...)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "run command %s %v, stderr=%s", argv0, args, stderr)
	}
	return strings.TrimSpace(stdout.String()), nil
}

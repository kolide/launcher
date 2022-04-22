/* Package make provides some simple functions to handle build and go
dependencies.

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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
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
	os                 string
	arch               string
	goVer              string
	goPath             string
	static             bool
	race               bool
	stampVersion       bool
	fakedata           bool
	notStripped        bool
	cgo                bool
	githubActionOutput bool

	cmdEnv []string
	execCC func(context.Context, string, ...string) *exec.Cmd
}

type Option func(*Builder)

func WithGoPath(goPath string) Option {
	return func(b *Builder) {
		b.goPath = goPath
	}
}

func WithOS(o string) Option {
	return func(b *Builder) {
		b.os = o
	}
}

func WithArch(a string) Option {
	return func(b *Builder) {
		b.arch = a
	}
}

func WithCgo() Option {
	return func(b *Builder) {
		b.cgo = true
	}
}

func WithStatic() Option {
	return func(b *Builder) {
		b.static = true
	}
}

func WithOutStripped() Option {
	return func(b *Builder) {
		b.notStripped = true
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

func WithFakeData() Option {
	return func(b *Builder) {
		b.fakedata = true
	}
}

func WithGithubActionOutput() Option {
	return func(b *Builder) {
		b.githubActionOutput = true
	}
}

func New(opts ...Option) *Builder {
	b := Builder{
		os:     runtime.GOOS,
		arch:   runtime.GOARCH,
		goPath: "go",
		goVer:  strings.TrimPrefix(runtime.Version(), "go"),

		execCC: exec.CommandContext,
	}

	for _, opt := range opts {
		opt(&b)
	}

	// Some default environment things
	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, "GO111MODULE=on")
	cmdEnv = append(cmdEnv, fmt.Sprintf("GOOS=%s", b.os))
	cmdEnv = append(cmdEnv, fmt.Sprintf("GOARCH=%s", b.arch))

	// CGO...
	switch {

	// https://github.com/kolide/launcher/pull/776 has a theory
	// that windows and cgo aren't friends. This might be wrong,
	// but I don't want to change it yet.
	case b.cgo && b.os == "windows":
		panic("Windows and CGO are not friends")

	// cgo is intentionally enabled
	case b.cgo:
		cmdEnv = append(cmdEnv, "CGO_ENABLED=1")

	// When cross compiling for ARCH, cgo is not automatically detected. So we force it here.
	case b.arch != runtime.GOARCH:
		cmdEnv = append(cmdEnv, "CGO_ENABLED=1")
	}

	// Setup zig as cross compiler for linux
	// (This is mostly to support fscrypt on linux)
	if b.os == "linux" && (b.os != runtime.GOOS || b.arch != runtime.GOARCH) {
		cwd, err := os.Getwd()
		if err != nil {
			// panic here feels a little uncouth, but the
			// caller here is a bunch simpler if we can
			// return *Builder, and this error is
			// exceedingly unlikely.
			panic(fmt.Sprintf("Unable to get cwd: %s", err))
		}

		cmdEnv = append(
			cmdEnv,
			fmt.Sprintf("ZIGTARGET=%s", zigTarget(b.os, b.arch)),
			fmt.Sprintf("CC=%s", filepath.Join(cwd, "tools", "zcc")),
			fmt.Sprintf("CXX=%s", filepath.Join(cwd, "tools", "zxx")),
		)
	}

	// I don't remember remember why we do this, but it might
	// break linux, as we need CGO for fscrypt
	if b.static {
		cmdEnv = append(cmdEnv, "CGO_ENABLED=0")
	}

	b.cmdEnv = cmdEnv

	return &b
}

func zigTarget(goos, goarch string) string {
	switch goarch {
	case "amd64":
		goarch = "x86_64"
	case "arm64":
		goarch = "aarch64"
	}

	if goos == "darwin" {
		goos = "macos"
	}

	return fmt.Sprintf("%s-%s", goarch, goos)
}

// PlatformBinaryName is a helper to return the platform specific output path.
func (b *Builder) PlatformBinaryName(input string) string {
	// On windows, everything must end in .exe. Strip off the extension
	// suffix, if present, and add .exe
	if b.os == "windows" {
		input = strings.TrimSuffix(input, ".ext") + ".exe"
	}

	platformName := fmt.Sprintf("%s.%s", b.os, b.arch)

	return filepath.Join("build", platformName, input)
}

func (b *Builder) goVersionCompatible(logger log.Logger) error {
	if b.goVer == "" {
		return errors.New("no go version. Is this a bad mock?")
	}

	if strings.HasPrefix(b.goVer, "devel") {
		level.Info(logger).Log(
			"msg", "Skipping version check for development version",
			"version", b.goVer,
		)
		return nil
	}

	goVer, err := semver.NewVersion(b.goVer)
	if err != nil {
		return errors.Wrapf(err, "parse go version %q as semver", b.goVer)
	}

	goConstraint := ">= 1.11"
	c, _ := semver.NewConstraint(goConstraint)
	if !c.Check(goVer) {
		return errors.Errorf("project requires Go version %s, have %s", goConstraint, goVer)
	}
	return nil
}

func (b *Builder) DepsGo(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "make.DepsGo")
	defer span.End()

	logger := ctxlog.FromContext(ctx)

	level.Debug(logger).Log(
		"cmd", "go mod download",
		"msg", "Starting",
	)

	if err := b.goVersionCompatible(logger); err != nil {
		return err
	}
	cmd := b.execCC(ctx, "go", "mod", "download")
	cmd.Env = append(cmd.Env, b.cmdEnv...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "run go mod download, output=%s", out)
	}

	level.Debug(logger).Log(
		"cmd", "go mod download",
		"msg", "Finished",
		"output", string(out),
	)

	return nil
}

func (b *Builder) InstallTools(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "make.InstallTools")
	defer span.End()

	logger := ctxlog.FromContext(ctx)

	level.Debug(logger).Log(
		"cmd", "Install Tools",
		"msg", "Starting",
	)

	cmd := b.execCC(
		ctx,
		"go", "list",
		"-tags", "tools",
		"-json",
		"./pkg/tools",
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

		g.Go(func() error {
			return b.installTool(ctx, toolPath)
		})
	}
	err = g.Wait()

	level.Debug(logger).Log(
		"cmd", "Install Tools",
		"msg", "Finished",
	)

	return errors.Wrap(err, "install tools")
}

func (b *Builder) installTool(ctx context.Context, importPath string) error {
	ctx, span := trace.StartSpan(ctx, "make.installTool")
	defer span.End()

	cmd := b.execCC(ctx, "go", "install", importPath)
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

	// First, we generate a bindata file from an empty directory so that the symbols
	// are present (Asset, AssetDir, etc). Once the symbols are present, we can run
	// the generate_tuf.go tool to generate actual TUF metadata. Finally, we recreate
	// the bindata file with the real TUF metadata.
	dir, err := ioutil.TempDir("", "bootstrap-launcher-bindata")
	if err != nil {
		return errors.Wrapf(err, "create empty dir for bindata")
	}
	defer os.RemoveAll(dir)

	if err := b.execBindata(ctx, dir); err != nil {
		return errors.Wrap(err, "exec bindata for empty dir")
	}

	binaryTargets := []string{ // binaries that are autoupdated.
		"osqueryd",
		"launcher",
	}

	// previous this depended on fs.Gopath to find the templated
	// notary files. As not everyone uses gopath, we make
	// assuptions about how this is called to find the template dir.
	// https://github.com/kolide/launcher/pull/503 is a better route.
	_, myFilename, _, _ := runtime.Caller(1)
	notaryConfigDir := filepath.Join(filepath.Dir(myFilename), "..", "..", "tools", "notary", "config")
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

	if err := b.execBindata(ctx, "pkg/autoupdate/assets/..."); err != nil {
		return errors.Wrap(err, "exec bindata for autoupdate assets")
	}

	return nil
}

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

func (b *Builder) BuildCmd(src, appName string) func(context.Context) error {
	return func(ctx context.Context) error {
		output := b.PlatformBinaryName(appName)

		ctx, span := trace.StartSpan(ctx, fmt.Sprintf("make.BuildCmd.%s", appName))
		defer span.End()

		logger := ctxlog.FromContext(ctx)

		level.Debug(logger).Log(
			"cmd", "Build",
			"app", appName,
			"msg", "Starting",
			"os", b.os,
			"arch", b.arch,
		)

		baseArgs := []string{"build", "-o", output}
		if b.race {
			baseArgs = append(baseArgs, "-race")
		}

		if b.fakedata {
			baseArgs = append(baseArgs, "-tags", "fakeserial")
		}

		var ldFlags []string
		if b.static {
			ldFlags = append(ldFlags, "-d -linkmode internal")
		}

		if !b.notStripped {
			ldFlags = append(ldFlags, "-w -s")
		}

		if b.stampVersion {
			v, err := b.getVersion(ctx)
			if err != nil {
				return errors.Wrap(err, "getVersion")
			}

			if b.fakedata {
				v = fmt.Sprintf("%s-fakedata", v)
			}

			branch, err := b.execOut(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return errors.Wrap(err, "git for branch")
			}

			revision, err := b.execOut(ctx, "git", "rev-parse", "HEAD")
			if err != nil {
				return errors.Wrap(err, "git for revision")
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

		// Set the build time for autoupdate.FindNewest
		ldFlags = append(ldFlags, fmt.Sprintf(`-X "github.com/kolide/launcher/pkg/autoupdate.defaultBuildTimestamp=%s"`, strconv.FormatInt(time.Now().Unix(), 10)))

		if len(ldFlags) != 0 {
			baseArgs = append(baseArgs, fmt.Sprintf("--ldflags=%s", strings.Join(ldFlags, " ")))
		}
		args := append(baseArgs, src)

		cmd := b.execCC(ctx, b.goPath, args...)
		cmd.Env = append(cmd.Env, b.cmdEnv...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		level.Debug(ctxlog.FromContext(ctx)).Log(
			"mgs", "building binary",
			"app_name", appName,
			"output", output,
			"go_args", strings.Join(args, "  "),
			"env", fmt.Sprintf("%v", cmd.Env),
		)

		if err := cmd.Run(); err != nil {
			return err
		}

		// Tell github where we're at
		if b.githubActionOutput {
			fmt.Printf("::set-output name=binary::%s\n", output)
		}

		// all the builds go to `build/<os>/binary`, but if the build OS is the same as the target OS,
		// we also want to hardlink the resulting binary at the root of `build/` for convenience.
		// ex: running ./build/launcher on macos instead of ./build/darwin/launcher
		if b.os == runtime.GOOS && b.arch == runtime.GOARCH {
			_, binName := filepath.Split(output)
			symlinkTarget := filepath.Join("build", binName)

			if err := os.Remove(symlinkTarget); err != nil && !os.IsNotExist(err) {
				// log but don't fail. This could happen if for example ./build/launcher.exe is referenced by a running service.
				// if this becomes clearer, we can either return an error here, or go back to silently ignoring.
				level.Debug(ctxlog.FromContext(ctx)).Log("msg", "remove before hardlink failed", "err", err, "app_name", appName)
			}
			return os.Link(output, symlinkTarget)
		}
		return nil
	}
}

// getVersion uses `git describe` to determine the version of the
// running code. The underlying functionality is as simple as
// strings.TrimPrefix, but there is some additional sanity checking
// with a regex.
func (b *Builder) getVersion(ctx context.Context) (string, error) {
	gitVersion, err := b.execOut(ctx, "git", "describe", "--tags", "--always", "--dirty")
	if err != nil {
		return "", errors.Wrap(err, "git describe")
	}

	// The `-` is included in the "additional" part of the regex,
	// to make the later concatenation correct. If, and when, we
	// move to a windows style 0.0.0.0 format, this will need to
	// change.
	versionRegex, err := regexp.Compile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:(-.+))?$`)
	if err != nil {
		return "", errors.Wrap(err, "bad regex")
	}

	// regex match and check the results
	matches := versionRegex.FindAllStringSubmatch(gitVersion, -1)

	if len(matches) == 0 {
		return "", errors.Errorf(`Version "%s" did not match expected format. Expect major.minor[.patch][-additional]`, gitVersion)
	}

	if len(matches[0]) != 5 {
		return "", errors.Errorf("Something very wrong. Expected 5 subgroups got %d from string %s", len(matches), gitVersion)
	}

	major := matches[0][1]
	minor := matches[0][2]
	patch := matches[0][3]
	additional := matches[0][4]

	if patch == "" {
		patch = "0"
	}

	version := fmt.Sprintf("%s.%s.%s%s", major, minor, patch, additional)

	return version, nil
}

func (b *Builder) execOut(ctx context.Context, argv0 string, args ...string) (string, error) {
	cmd := b.execCC(ctx, argv0, args...)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "run command %s %v, stderr=%s", argv0, args, stderr)
	}
	return strings.TrimSpace(stdout.String()), nil
}

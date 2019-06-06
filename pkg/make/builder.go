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
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sync/errgroup"
)

type Builder struct {
	os           string
	arch         string
	static       bool
	race         bool
	stampVersion bool
	fakedata     bool

	goVer  *semver.Version
	cmdEnv []string
	execCC func(context.Context, string, ...string) *exec.Cmd
}

type Option func(*Builder)

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

func WithFakeData() Option {
	return func(b *Builder) {
		b.fakedata = true
	}
}

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
	if b.static {
		cmdEnv = append(cmdEnv, "CGO_ENABLED=0")
	}

	b.cmdEnv = cmdEnv

	return &b, nil

}

// PlatformExtensionName is a helper to return the platform specific extension name.
func (b *Builder) PlatformExtensionName(input string) string {
	input = filepath.Join("build", b.os, input)
	if b.os == "windows" {
		return input + ".exe"
	} else {
		return input + ".ext"
	}
}

// PlatformBinaryName is a helper to return the platform specific binary suffix.
func (b *Builder) PlatformBinaryName(input string) string {
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

	logger := ctxlog.FromContext(ctx)

	level.Debug(logger).Log(
		"cmd", "go mod download",
		"msg", "Starting",
	)

	if err := b.goVersionCompatible(); err != nil {
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

func (b *Builder) BuildCmd(src, output string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, appName := filepath.Split(output)

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
			ldFlags = append(ldFlags, "-w -d -linkmode internal")
		}
		if b.stampVersion {
			v, err := b.execOut(ctx, "git", "describe", "--tags", "--always", "--dirty")
			if err != nil {
				return err
			}

			if b.fakedata {
				v = fmt.Sprintf("%s-fakedata", v)
			}

			branch, err := b.execOut(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return err
			}

			revision, err := b.execOut(ctx, "git", "rev-parse", "HEAD")
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

		cmd := b.execCC(ctx, "go", args...)
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

func (b *Builder) execOut(ctx context.Context, argv0 string, args ...string) (string, error) {
	cmd := b.execCC(ctx, argv0, args...)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "run command %s %v, stderr=%s", argv0, args, stderr)
	}
	return strings.TrimSpace(stdout.String()), nil
}

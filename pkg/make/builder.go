/*
Package make provides some simple functions to handle build and go
dependencies.

We used to do this with gnumake rules, but as we added windows
compatibility, we found make too limiting. Moving this into go allows
us to write cleaner cross-platform code.
*/
package make //nolint:predeclared

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
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

		execCC: exec.CommandContext, //nolint:forbidigo // Fine to use exec.CommandContext outside of launcher proper
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
		panic("Windows and CGO are not friends") //nolint:forbidigo // Fine to use panic outside of launcher proper

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
			panic(fmt.Sprintf("Unable to get cwd: %s", err)) //nolint:forbidigo // Fine to use panic outside of launcher proper
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
		return fmt.Errorf("parse go version %q as semver: %w", b.goVer, err)
	}

	goConstraint := ">= 1.11"
	c, _ := semver.NewConstraint(goConstraint)
	if !c.Check(goVer) {
		return fmt.Errorf("project requires Go version %s, have %s", goConstraint, goVer)
	}
	return nil
}

func (b *Builder) DepsGo(ctx context.Context) error {
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
		return fmt.Errorf("run go mod download, output=%s: %w", out, err)
	}

	level.Debug(logger).Log(
		"cmd", "go mod download",
		"msg", "Finished",
		"output", string(out),
	)

	return nil
}

func (b *Builder) BuildCmd(src, appName string) func(context.Context) error {
	return func(ctx context.Context) error {
		output := b.PlatformBinaryName(appName)

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
		} else if b.os == "linux" && b.arch != runtime.GOARCH {
			// Cross-compiling for Linux requires external linking
			ldFlags = append(ldFlags, "-linkmode external")
		}

		if !b.notStripped {
			ldFlags = append(ldFlags, "-w -s")
		} else {
			level.Info(logger).Log(
				"msg", "compiling debug build without stripping symbols",
				"os", b.os,
				"arch", b.arch,
			)
			// In the future, we may want to also append `-gcflags=all=-N -l`.
			// -N disables optimizations, and -l disables inlining, so they may give us improved debugging.
			// For now, though, we don't include them because they cause the following error on Windows ARM:
			// "syscall.Syscall15: nosplit stack over 792 byte limit".
		}

		if b.os == "windows" {
			// this prevents a cmd prompt opening up when desktop is launched
			ldFlags = append(ldFlags, "-H windowsgui")
		}

		if b.os == "darwin" {
			// Suppress warnings like "ld: warning: ignoring duplicate libraries: '-lobjc'"
			// See: https://github.com/golang/go/issues/67799
			ldFlags = append(ldFlags, "-extldflags=-Wl,-no_warn_duplicate_libraries")
		}

		if b.stampVersion {
			v, err := b.getVersion(ctx)
			if err != nil {
				return fmt.Errorf("getVersion: %w", err)
			}

			if b.fakedata {
				v = fmt.Sprintf("%s-fakedata", v)
			}

			branch, err := b.execOut(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return fmt.Errorf("git for branch: %w", err)
			}

			revision, err := b.execOut(ctx, "git", "rev-parse", "HEAD")
			if err != nil {
				return fmt.Errorf("git for revision: %w", err)
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

		cmd := b.execCC(ctx, b.goPath, args...)
		cmd.Env = append(cmd.Env, b.cmdEnv...)
		cmd.Stdout = os.Stdout

		// Some build commands, especially relating to link errors on macOS, are emitted to STDERR and do not change
		// exit code. To compensate, we can capture stderr, and if present, declare the run a failure. This was prompted
		// by https://github.com/kolide/launcher/issues/1276
		var stderr bytes.Buffer
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

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

		// With the Sonoma-era Xcode binaries, we've started seeing a bunch of spurious warnings. They appear to
		// _mostly_ effect our developer build process. In the interest in not breaking the build, we ignore them.
		// https://github.com/golang/go/issues/62597#issuecomment-1733893918 is some of them, and some seem related to
		// the zig cross compiling
		stderrStr := stderr.String()
		if os.Getenv("GITHUB_ACTIONS") == "" {
			stderrStr = strings.ReplaceAll(stderrStr, "ld: warning: ignoring duplicate libraries: '-lobjc'\n", "")
			stderrStr = strings.ReplaceAll(stderrStr, fmt.Sprintf("# github.com/kolide/launcher/cmd/%s\n", filepath.Base(src)), "")

			re := regexp.MustCompile(`ld: warning: object file \(.*\) was built for newer 'macOS' version \(.+\) than being linked \(.+\)\n`)
			stderrStr = re.ReplaceAllString(stderrStr, "")
		}

		if len(stderrStr) > 0 {
			return errors.New("stderr not empty")
		}

		// Tell github where we're at
		if b.githubActionOutput {
			outputFilePath := os.Getenv("GITHUB_OUTPUT")

			f, err := os.OpenFile(outputFilePath, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("failed to open $GITHUB_OUTPUT file: %w", err)
			}

			defer func() {
				if err := f.Close(); err != nil {
					level.Error(ctxlog.FromContext(ctx)).Log(
						"mgs", "Got Error writing GITHUB_OUTPUT",
						"app_name", appName,
						"err", err,
					)
				}
			}()

			if _, err = f.WriteString(fmt.Sprintf("binary=%s\n", output)); err != nil {
				return fmt.Errorf("failed to write to $GITHUB_OUTPUT file: %w", err)
			}
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
		return "", fmt.Errorf("git describe: %w", err)
	}

	// The `-` is included in the "additional" part of the regex,
	// to make the later concatenation correct. If, and when, we
	// move to a windows style 0.0.0.0 format, this will need to
	// change.
	versionRegex, err := regexp.Compile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:(-.+))?$`)
	if err != nil {
		return "", fmt.Errorf("bad regex: %w", err)
	}

	// regex match and check the results
	matches := versionRegex.FindAllStringSubmatch(gitVersion, -1)

	if len(matches) == 0 {
		return "", fmt.Errorf(`version "%s" did not match expected format: expected major.minor[.patch][-additional]`, gitVersion)
	}

	if len(matches[0]) != 5 {
		return "", fmt.Errorf("something very wrong: expected 5 subgroups, got %d, from string %s", len(matches), gitVersion)
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
		return "", fmt.Errorf("run command %s %v, stderr=%s: %w", argv0, args, stderr, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

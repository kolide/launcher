package packagekit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
)

type outputType string

const (
	Deb    outputType = "deb"
	RPM    outputType = "rpm"
	Tar    outputType = "tar"
	Pacman outputType = "pacman"
)

type fpmOptions struct {
	outputType outputType
	replaces   []string
	arch       string
}

type FpmOpt func(*fpmOptions)

func AsRPM() FpmOpt {
	return func(f *fpmOptions) {
		f.outputType = RPM
	}
}

func AsDeb() FpmOpt {
	return func(f *fpmOptions) {
		f.outputType = Deb
	}
}

func AsTar() FpmOpt {
	return func(f *fpmOptions) {
		f.outputType = Tar
	}
}

func AsPacman() FpmOpt {
	return func(f *fpmOptions) {
		f.outputType = Pacman
	}
}

// WithReplaces passes a list of package names tpo fpm's replace and
// conflict options. This allows creation of packages that supercede
// previous versions.
func WithReplaces(r []string) FpmOpt {
	return func(f *fpmOptions) {
		f.replaces = r
	}
}

func WithArch(arch string) FpmOpt {
	return func(f *fpmOptions) {
		f.arch = arch
	}
}

func PackageFPM(ctx context.Context, w io.Writer, po *PackageOptions, fpmOpts ...FpmOpt) error {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "packagekit.PackageFPM")

	f := fpmOptions{}
	for _, opt := range fpmOpts {
		opt(&f)
	}

	if f.outputType == "" {
		return errors.New("missing output type")
	}

	if f.arch == "" {
		return errors.New("missing architecture")
	}

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	outputFilename := fmt.Sprintf("%s-%s.%s", po.Name, po.Version, f.outputType)

	outputPathDir, err := os.MkdirTemp("", "packaging-fpm-output")
	if err != nil {
		return fmt.Errorf("making TempDir: %w", err)
	}
	defer os.RemoveAll(outputPathDir)

	// Set arch correctly when invoking fpm. Allowable values are amd64 (does not require update) and
	// aarch64 (requires update from "arm64").
	arch := f.arch
	if arch == "arm64" {
		arch = "aarch64"
	}

	fpmCommand := []string{
		"fpm",
		"-s", "dir",
		"-t", string(f.outputType),
		"-n", fmt.Sprintf("%s-%s", po.Name, po.Identifier),
		"-v", po.Version,
		"-a", arch,
		"-p", filepath.Join("/out", outputFilename),
		"-C", "/pkgsrc",
	}

	if f.outputType == Pacman {
		fpmCommand = append(fpmCommand, "--pacman-compression", "gz")
	}

	// Pass each replaces in. Set it as a conflict and a replace.
	for _, r := range f.replaces {
		fpmCommand = append(fpmCommand, "--replaces", r, "--conflicts", r)
	}

	// If postinstall exists, pass it to fpm
	if _, err := os.Stat(filepath.Join(po.Scripts, "postinstall")); !os.IsNotExist(err) {
		fpmCommand = append(fpmCommand, "--after-install", filepath.Join("/pkgscripts", "postinstall"))
	}

	// If prerm exists, pass it to fpm
	if _, err := os.Stat(filepath.Join(po.Scripts, "prerm")); !os.IsNotExist(err) {
		fpmCommand = append(fpmCommand, "--before-remove", filepath.Join("/pkgscripts", "prerm"))
	}

	mountSuffix := ""
	if po.ContainerTool == "podman" {
		// private volume, necessary to avoid permission issues when building rootlessly
		mountSuffix = ":Z"
	}

	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/pkgsrc%s", po.Root, mountSuffix),
		"-v", fmt.Sprintf("%s:/pkgscripts%s", po.Scripts, mountSuffix),
		"-v", fmt.Sprintf("%s:/out%s", outputPathDir, mountSuffix),
		"--entrypoint", "", // override this, to ensure more compatibility with the plain command line
		"docker.io/kolide/fpm:latest",
	}

	cmd := exec.CommandContext(ctx, po.ContainerTool, append(args, fpmCommand...)...) //nolint:forbidigo // Fine to use exec.CommandContext outside of launcher proper

	stderr := new(bytes.Buffer)
	stdout := new(bytes.Buffer)
	cmd.Stderr = stderr
	cmd.Stdout = stdout

	level.Debug(logger).Log(
		"msg", "Running fpm",
		"cmd", strings.Join(cmd.Args, " "),
	)

	if err := cmd.Run(); err != nil {
		level.Error(logger).Log(
			"msg", "Error running fpm",
			"err", err,
			"cmd", strings.Join(cmd.Args, " "),
			"stderr", stderr,
			"stdout", stdout,
		)
		return fmt.Errorf("creating fpm package: %s: %w", stderr, err)
	}
	level.Debug(logger).Log("msg", "fpm exited cleanly")

	outputFH, err := os.Open(filepath.Join(outputPathDir, outputFilename))
	if err != nil {
		return fmt.Errorf("opening resultant output file: %w", err)
	}
	defer outputFH.Close()

	level.Debug(logger).Log("msg", "Copying fpm built to remote filehandle")
	if _, err := io.Copy(w, outputFH); err != nil {
		return fmt.Errorf("copying output: %w", err)
	}

	SetInContext(ctx, ContextLauncherVersionKey, po.Version)

	return nil
}

package packagekit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

type outputType string

const (
	Deb    outputType = "deb"
	RPM               = "rpm"
	Tar               = "tar"
	Pacman            = "pacman"
)

type fpmOptions struct {
	outputType outputType
	replaces   []string
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

func PackageFPM(ctx context.Context, w io.Writer, po *PackageOptions, fpmOpts ...FpmOpt) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackageRPM")
	defer span.End()
	logger := log.With(ctxlog.FromContext(ctx), "caller", "packagekit.PackageFPM")

	f := fpmOptions{}
	for _, opt := range fpmOpts {
		opt(&f)
	}

	if f.outputType == "" {
		return errors.New("Missing output type")
	}

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	outputFilename := fmt.Sprintf("%s-%s.%s", po.Name, po.Version, f.outputType)

	outputPathDir, err := ioutil.TempDir("", "packaging-fpm-output")
	if err != nil {
		return errors.Wrap(err, "making TempDir")
	}
	defer os.RemoveAll(outputPathDir)

	fpmCommand := []string{
		"fpm",
		"-s", "dir",
		"-t", string(f.outputType),
		"-n", fmt.Sprintf("%s-%s", po.Name, po.Identifier),
		"-v", po.Version,
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

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/pkgsrc", po.Root),
		"-v", fmt.Sprintf("%s:/pkgscripts", po.Scripts),
		"-v", fmt.Sprintf("%s:/out", outputPathDir),
		"--entrypoint", "", // override this, to ensure more compatibility with the plain command line
		"kolide/fpm:latest",
	}

	cmd := exec.CommandContext(ctx, "docker", append(dockerArgs, fpmCommand...)...)

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
		return errors.Wrapf(err, "creating fpm package: %s", stderr)
	}
	level.Debug(logger).Log("msg", "fpm exited cleanly")

	outputFH, err := os.Open(filepath.Join(outputPathDir, outputFilename))
	if err != nil {
		return errors.Wrap(err, "opening resultant output file")
	}
	defer outputFH.Close()

	level.Debug(logger).Log("msg", "Copying fpm built to remote filehandle")
	if _, err := io.Copy(w, outputFH); err != nil {
		return errors.Wrap(err, "copying output")
	}

	return nil
}

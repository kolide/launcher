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

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

type outputType string

const (
	Deb outputType = "deb"
	RPM            = "rpm"
	Tar            = "tar"
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

	outputPathDir, err := ioutil.TempDir("/tmp", "packaging-fpm-output")
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
		"kolide/fpm",
	}

	cmd := exec.CommandContext(ctx, "docker", append(dockerArgs, fpmCommand...)...)

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "creating fpm package: %s", stderr)
	}

	outputFH, err := os.Open(filepath.Join(outputPathDir, outputFilename))
	if err != nil {
		return errors.Wrap(err, "opening resultant output file")
	}
	defer outputFH.Close()

	if _, err := io.Copy(w, outputFH); err != nil {
		return errors.Wrap(err, "copying output")
	}

	return nil
}

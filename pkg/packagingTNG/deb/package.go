package deb

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

type PackageOptions struct {
	Version      string
	AfterInstall string // postinstall script to run.
}

type Option func(*PackageOptions)

func WithVersion(v string) Option {
	return func(o *PackageOptions) {
		o.Version = v
	}
}

func WithAfterInstall(s string) Option {
	return func(o *PackageOptions) {
		o.AfterInstall = s
	}
}

func Package(w io.Writer, name string, packageRoot string, opts ...Option) error {
	options := &PackageOptions{
		Version: "0.0.0",
	}

	for _, opt := range opts {
		opt(options)
	}

	if packageRootStat, err := os.Stat(packageRoot); os.IsNotExist(err) {
		return errors.Wrapf(err, "missing packageRoot %s", packageRoot)
	} else {
		if !packageRootStat.IsDir() {
			return errors.Errorf("packageRoot (%s) isn't a directory", packageRoot)
		}
	}

	outputFilename := fmt.Sprintf("%s-%s.deb", name, options.Version)

	outputPathDir, err := ioutil.TempDir("/tmp", "packaging-deb-output")
	if err != nil {
		return errors.Wrap(err, "making TempDir")
	}
	defer os.RemoveAll(outputPathDir)

	fpmCommand := []string{
		"fpm",
		"-s", "dir",
		"-t", "deb",
		"-n", name,
		"-v", options.Version,
		"-p", filepath.Join("/out", outputFilename),
		"-C", "/pkgsrc",
	}

	if options.AfterInstall != "" {
		fpmCommand = append(fpmCommand, "--after-install", options.AfterInstall)
	}

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/pkgsrc", packageRoot),
		"-v", fmt.Sprintf("%s:/out", outputPathDir),
		"kolide/fpm",
	}

	cmd := exec.Command("docker", append(dockerArgs, fpmCommand...)...)

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "creating deb package: %s", stderr)
	}

	outputFH, err := os.Open(filepath.Join(outputPathDir, outputFilename))
	if err != nil {
		return errors.Wrap(err, "opening resultant output file")
	}

	if _, err := io.Copy(w, outputFH); err != nil {
		return errors.Wrap(err, "copying output")
	}

	return nil
}

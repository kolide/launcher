package pkg

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
	SigningKey   string
}

type Option func(*PackageOptions)

func WithVersion(v string) Option {
	return func(o *PackageOptions) {
		o.Version = v
	}
}

func WithSigningKey(k string) Option {
	return func(o *PackageOptions) {
		o.SigningKey = k
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

	outputFilename := fmt.Sprintf("%s-%s.pkg", name, options.Version)

	outputPathDir, err := ioutil.TempDir("", "packaging-pkg-output")
	if err != nil {
		return errors.Wrap(err, "making TempDir")
	}
	defer os.RemoveAll(outputPathDir)

	outputPath := filepath.Join(outputPathDir, outputFilename)

	// Setup the script dir
	scriptsDir, err := ioutil.TempDir("", "packaging-pkg-script")
	if err != nil {
		return errors.Wrap(err, "could not create temp directory for the macOS packaging script directory")
	}
	defer os.RemoveAll(scriptsDir)

	/*
		postinstallFile, err := os.Create(filepath.Join(scriptDir, "postinstall"))
		if err != nil {
			return errors.Wrap(err, "opening the postinstall file for writing")
		}
		if err := postinstallFile.Chmod(0755); err != nil {
			return errors.Wrap(err, "could not make postinstall script executable")
		}
	*/

	args := []string{
		"--root", packageRoot,
		"--scripts", scriptsDir,
		"--identifier", name, // FIXME? identifier,
		"--version", options.Version,
	}

	if options.SigningKey != "" {
		args = append(args, "--sign", options.SigningKey)
	}

	args = append(args, outputPath)

	cmd := exec.Command("pkgbuild", args...)

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "creating pkg package: %s", stderr)
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

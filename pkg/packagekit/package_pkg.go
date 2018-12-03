package packagekit

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

type PkgOptions struct {
	SigningKey string
}

type PkgOption func(*PkgOptions)

func WithSigningKey(k string) PkgOption {
	return func(o *PkgOptions) {
		o.SigningKey = k
	}
}

func PackagePkg(w io.Writer, po *PackageOptions, opts ...PkgOption) error {
	options := &PkgOptions{}

	for _, opt := range opts {
		opt(options)
	}

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	outputFilename := fmt.Sprintf("%s-%s.pkg", po.Name, po.Version)

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
		"--root", po.Root,
		"--scripts", scriptsDir,
		"--identifier", po.Name, // FIXME? identifier,
		"--version", po.Version,
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

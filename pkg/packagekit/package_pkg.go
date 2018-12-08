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

func PackagePkg(ctx context.Context, w io.Writer, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackagePkg")
	defer span.End()

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

	// pkg ship a scripts dir _outside_ the package root. this is bundled at packaging time.
	scriptsDir, err := ioutil.TempDir("", "packaging-pkg-script")
	if err != nil {
		return errors.Wrap(err, "could not create temp directory for the macOS packaging script directory")
	}
	defer os.RemoveAll(scriptsDir)

	if po.Postinst != nil {
		postinstallFile, err := os.Create(filepath.Join(scriptsDir, "postinstall"))
		if err != nil {
			return errors.Wrap(err, "opening the postinstall file for writing")
		}
		defer postinstallFile.Close()
		if err := postinstallFile.Chmod(0755); err != nil {
			return errors.Wrap(err, "could not make postinstall script executable")
		}
		if _, err := io.Copy(postinstallFile, po.Postinst); err != nil {
			return errors.Wrap(err, "could not write postinstall script")
		}
	}

	args := []string{
		"--root", po.Root,
		"--scripts", scriptsDir,
		"--identifier", po.Name, // FIXME? identifier,
		"--version", po.Version,
	}

	if po.SigningKey != "" {
		args = append(args, "--sign", po.SigningKey)
	}

	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, "pkgbuild", args...)

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

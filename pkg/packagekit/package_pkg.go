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

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

func PackagePkg(ctx context.Context, w io.Writer, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackagePkg")
	defer span.End()

	logger := ctxlog.FromContext(ctx)

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

	args := []string{
		"--root", po.Root,
		"--identifier", fmt.Sprintf("com.%s.launcher", po.Identifier),
		"--version", po.Version,
	}

	if po.Scripts != "" {
		args = append(args, "--scripts", po.Scripts)
	}

	if po.SigningKey != "" {
		args = append(args, "--sign", po.SigningKey)
	}

	args = append(args, outputPath)

	level.Debug(logger).Log(
		"msg", "Running pkbuild",
		"args", args,
	)

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

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

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/packagekit/applenotarization"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

func PackagePkg(ctx context.Context, w io.Writer, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackagePkg")
	defer span.End()

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	outputPathDir, err := ioutil.TempDir("", "packaging-pkg-output")
	if err != nil {
		return errors.Wrap(err, "making TempDir")
	}
	defer os.RemoveAll(outputPathDir)

	flatPkgPath := filepath.Join(outputPathDir, fmt.Sprintf("%s-%s.flat.pkg", po.Name, po.Version))
	distributionPkgPath := filepath.Join(outputPathDir, fmt.Sprintf("%s-%s.pkg", po.Name, po.Version))

	if err := runPkbuild(ctx, flatPkgPath, po); err != nil {
		return errors.Wrap(err, "running pkgbuild")
	}

	if err := runProductbuild(ctx, flatPkgPath, distributionPkgPath, po); err != nil {
		return errors.Wrap(err, "running productbuild")
	}

	if err := runNotarize(ctx, distributionPkgPath, po); err != nil {
		return errors.Wrap(err, "running notarize")
	}

	outputFH, err := os.Open(distributionPkgPath)
	if err != nil {
		return errors.Wrap(err, "opening resultant output file")
	}
	defer outputFH.Close()

	if _, err := io.Copy(w, outputFH); err != nil {
		return errors.Wrap(err, "copying output")
	}

	setInContext(ctx, ContextLauncherVersionKey, po.Version)

	return nil
}

// runNotarize takes a given input, and notarizes it
func runNotarize(ctx context.Context, file string, po *PackageOptions) error {
	if po.AppleNotarizeUserId == "" || po.AppleNotarizeAppPassword == "" {
		return nil
	}

	ctx, span := trace.StartSpan(ctx, "packagekit.runNotarize")
	defer span.End()

	logger := log.With(ctxlog.FromContext(ctx), "method", "packagekit.runNotarize")

	bundleid := fmt.Sprintf("com.%s.launcher", po.Identifier)
	notarizer := applenotarization.New(po.AppleNotarizeUserId, po.AppleNotarizeAppPassword, po.AppleNotarizeAccountId)
	uuid, err := notarizer.Submit(ctx, file, bundleid)
	if err != nil {
		return errors.Wrap(err, "submitting file for notarization")
	}

	level.Debug(logger).Log(
		"msg", "Got uuid",
		"uuid", uuid,
	)

	setInContext(ctx, ContextNotarizationUuidKey, uuid)

	return nil
}

// runPkbuild produces a flat pkg file. It uses the directories
// specified in PackageOptions, and then execs pkgbuild
func runPkbuild(ctx context.Context, outputPath string, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.runPkbuild")
	defer span.End()

	logger := log.With(ctxlog.FromContext(ctx), "method", "packagekit.runPkbuild")

	args := []string{
		"--root", po.Root,
		"--identifier", fmt.Sprintf("com.%s.launcher", po.Identifier),
		"--version", po.Version,
	}

	if po.Scripts != "" {
		args = append(args, "--scripts", po.Scripts)
	}

	if po.AppleSigningKey != "" {
		args = append(args, "--sign", po.AppleSigningKey)
	}

	args = append(args, outputPath)

	level.Debug(logger).Log(
		"msg", "Running pkgbuild",
		"args", fmt.Sprintf("%v", args),
	)

	cmd := exec.CommandContext(ctx, "pkgbuild", args...)

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "creating flat pkg package: %s", stderr)
	}

	return nil
}

// runProductbuild converts a flat package to a distribution
// package. It does this by execing productbuild.
//
// See https://github.com/kolide/launcher/issues/407 and associated links
func runProductbuild(ctx context.Context, flatPkgPath, distributionPkgPath string, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.runProductbuild")
	defer span.End()

	logger := log.With(ctxlog.FromContext(ctx), "method", "packagekit.runProductbuild")

	args := []string{}

	if po.AppleSigningKey != "" {
		args = append(args, "--sign", po.AppleSigningKey)
	}

	args = append(args,
		"--package", flatPkgPath,
		distributionPkgPath,
	)

	level.Debug(logger).Log(
		"msg", "Running productbuild",
		"args", fmt.Sprintf("%v", args),
	)

	cmd := exec.CommandContext(ctx, "productbuild", args...)

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "creating distribution pkg package: %s", stderr)
	}

	return nil
}

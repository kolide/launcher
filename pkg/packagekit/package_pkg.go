package packagekit

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/packagekit/applenotarization"

	"go.opencensus.io/trace"
)

//go:embed assets/distribution.dist
var distributionTemplate []byte

func PackagePkg(ctx context.Context, w io.Writer, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackagePkg")
	defer span.End()

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	outputPathDir, err := ioutil.TempDir("", "packaging-pkg-output")
	if err != nil {
		return fmt.Errorf("making TempDir: %w", err)
	}
	defer os.RemoveAll(outputPathDir)

	flatPkgPath := filepath.Join(outputPathDir, fmt.Sprintf("%s-%s.flat.pkg", po.Name, po.Version))
	distributionPkgPath := filepath.Join(outputPathDir, fmt.Sprintf("%s-%s.pkg", po.Name, po.Version))

	if err := runPkbuild(ctx, flatPkgPath, po); err != nil {
		return fmt.Errorf("running pkgbuild: %w", err)
	}

	if err := runProductbuild(ctx, flatPkgPath, distributionPkgPath, po); err != nil {
		return fmt.Errorf("running productbuild: %w", err)
	}

	if err := runNotarize(ctx, distributionPkgPath, po); err != nil {
		return fmt.Errorf("running notarize: %w", err)
	}

	outputFH, err := os.Open(distributionPkgPath)
	if err != nil {
		return fmt.Errorf("opening resultant output file: %w", err)
	}
	defer outputFH.Close()

	if _, err := io.Copy(w, outputFH); err != nil {
		return fmt.Errorf("copying output: %w", err)
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
		return fmt.Errorf("submitting file for notarization: %w", err)
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

	// Run analyze to generate our component plist
	componentPlist := "./launcher.plist"
	analyzeCmd := exec.CommandContext(ctx, "pkgbuild", "--analyze", "--root", po.Root, componentPlist)
	if err := analyzeCmd.Run(); err != nil {
		return fmt.Errorf("running analyze: %w", err)
	}

	// Clean up the newly-generated component plist after we're done with it
	defer func() {
		if err := os.Remove(componentPlist); err != nil {
			level.Error(logger).Log(
				"msg", "could not clean up component plist after pkgbuild",
				"plist", componentPlist,
			)
		}
	}()

	// Set BundleIsRelocatable in the component plist to false -- this makes sure that the installer
	// will install Kolide.app to the location that we expect
	replaceCmd := exec.CommandContext(ctx, "plutil", "-replace", "BundleIsRelocatable", "-bool", "false", componentPlist)
	if err := replaceCmd.Run(); err != nil {
		return fmt.Errorf("running plutil -replace: %w", err)
	}

	args := []string{
		"--root", po.Root,
		"--component-plist", componentPlist,
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
		return fmt.Errorf("creating flat pkg package: %s: %w", stderr, err)
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

	// Create a distribution file so that we can set the title and the minimum OS version
	distributionFile := "./distribution.dist"
	fh, err := os.Create(distributionFile)
	if err != nil {
		return fmt.Errorf("could not create distribution file %s: %w", distributionFile, err)
	}
	defer fh.Close()

	var templateData = struct {
		Title      string
		Identifier string
		Version    string
		PkgName    string
	}{
		Title:      po.Title,
		Identifier: po.Identifier,
		Version:    po.Version,
		PkgName:    filepath.Base(flatPkgPath),
	}
	t, err := template.New("distribution").Parse(string(distributionTemplate))
	if err != nil {
		return fmt.Errorf("could not parse distribution template: %w", err)
	}
	err = t.ExecuteTemplate(fh, "distribution", templateData)
	if err != nil {
		return fmt.Errorf("could not write distribution file: %w", err)
	}

	// Clean up the newly-generated distribution file after we're done with it
	defer func() {
		if err := os.Remove(distributionFile); err != nil {
			level.Error(logger).Log(
				"msg", "could not clean up distribution.dist after productbuild",
				"dist", distributionFile,
			)
		}
	}()

	args := []string{
		"--distribution", distributionFile,
	}

	if po.AppleSigningKey != "" {
		args = append(args, "--sign", po.AppleSigningKey)
	}

	args = append(args,
		"--package-path", filepath.Dir(flatPkgPath),
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
		return fmt.Errorf("creating distribution pkg package: %s: %w", stderr, err)
	}

	return nil
}

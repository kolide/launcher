package wix

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

type wixOptions struct {
	wixPath        string   // Where is wix installed
	packageRoot    string   // What's the root of the packaging files?
	buildDir       string   // The wix tools want to work in a build dir.
	msArch         string   // What's the microsoft archtecture name?
	services       []string // array of services. TBD
	dockerImage    string   // If in docker, what image?
	skipValidation bool     // Skip light validation. Seems to be needed for running in 32bit wine environments.
	cleanDirs      []string // directories to rm on cleanup

	execCC func(context.Context, string, ...string) *exec.Cmd // Allows test overrides
}

type WixOpt func(*wixOptions)

func As64bit() WixOpt {
	return func(wo *wixOptions) {
		wo.msArch = "x64"
	}
}

func As32bit() WixOpt {
	return func(wo *wixOptions) {
		wo.msArch = "x86"
	}
}

// If you're running this in a virtual win environment, you probably
// need to skip validation. LGHT0216 is a common error.
func SkipValidation() WixOpt {
	return func(wo *wixOptions) {
		wo.skipValidation = true
	}
}

func WithWix(path string) WixOpt {
	return func(wo *wixOptions) {
		wo.wixPath = path
	}
}

func WithServices(service string) WixOpt {
	return func(wo *wixOptions) {
		wo.services = append(wo.services, service)
	}
}

func WithBuildDir(path string) WixOpt {
	return func(wo *wixOptions) {
		wo.buildDir = path
	}
}

func WithDocker(image string) WixOpt {
	return func(wo *wixOptions) {
		wo.dockerImage = image

	}
}

// New takes a packageRoot of files, and a wxsContent of xml wix
// configs, and will return a struct suitable for builing packages
// with.
func New(packageRoot string, mainWxsContent []byte, wixOpts ...WixOpt) (*wixOptions, error) {
	wo := &wixOptions{
		wixPath:     `C:\wix311`,
		packageRoot: packageRoot,

		execCC: exec.CommandContext,
	}

	for _, opt := range wixOpts {
		opt(wo)
	}

	var err error
	if wo.buildDir == "" {
		wo.buildDir, err = ioutil.TempDir("", "wix-build-dir")
		if err != nil {
			return nil, errors.Wrap(err, "making temp wix-build-dir")
		}
		wo.cleanDirs = append(wo.cleanDirs, wo.buildDir)
	}

	if wo.msArch == "" {
		switch runtime.GOARCH {
		case "386":
			wo.msArch = "x86"
		case "amd64":
			wo.msArch = "x64"
		default:
			return nil, errors.Errorf("unknown arch for windows %s", runtime.GOARCH)
		}
	}

	mainWxsPath := filepath.Join(wo.buildDir, "Installer.wxs")

	if err := ioutil.WriteFile(
		mainWxsPath,
		mainWxsContent,
		0644); err != nil {
		return nil, errors.Wrapf(err, "writing %s", mainWxsPath)
	}

	return wo, nil
}

// Cleanup removes temp directories. Meant to be called in a defer.
func (wo *wixOptions) Cleanup() {
	for _, d := range wo.cleanDirs {
		os.RemoveAll(d)
	}
}

// Package will run through the wix steps to produce a resulting
// package. This package will be written into the provided io.Writer,
// facilitating export to a file, buffer, or other storage backends.
func (wo *wixOptions) Package(ctx context.Context, pkgOutput io.Writer) error {
	if err := wo.heat(ctx); err != nil {
		return errors.Wrap(err, "running heat")
	}

	if err := wo.candle(ctx); err != nil {
		return errors.Wrap(err, "running candle")
	}

	if err := wo.light(ctx); err != nil {
		return errors.Wrap(err, "running light")
	}

	msiFH, err := os.Open(filepath.Join(wo.buildDir, "out.msi"))
	if err != nil {
		return errors.Wrap(err, "opening msi output file")
	}
	defer msiFH.Close()

	if _, err := io.Copy(pkgOutput, msiFH); err != nil {
		return errors.Wrap(err, "copying output")
	}

	return nil
}

// heat invokes wix's heat command. This examines a directory and
// "harvests" the files into an xml structure. See
// http://wixtoolset.org/documentation/manual/v3/overview/heat.html
//
// TODO split this into PROGDIR and DATADIR. Perhaps using options? Or
// figuring out a way to invoke this multiple times with different dir
// and -cg settings.
func (wo *wixOptions) heat(ctx context.Context) error {
	_, err := wo.execOut(ctx,
		filepath.Join(wo.wixPath, "heat.exe"),
		"dir", wo.packageRoot,
		"-nologo",
		"-gg", "-g1",
		"-srd",
		"-sfrag",
		"-ke",
		"-cg", "AppFiles",
		"-template", "fragment",
		"-dr", "DATADIR",
		"-var", "var.SourceDir",
		"-out", "AppFiles.wxs",
	)
	return err
}

// candle invokes wix's candle command. This is the wix compiler, It
// preprocesses and compiles WiX source files into object files
// (.wixobj).
func (wo *wixOptions) candle(ctx context.Context) error {
	_, err := wo.execOut(ctx,
		filepath.Join(wo.wixPath, "candle.exe"),
		"-nologo",
		"-arch", wo.msArch,
		"-dSourceDir="+wo.packageRoot,
		"Installer.wxs",
		"AppFiles.wxs",
	)
	return err
}

// light invokes wix's light command. This links and binds one or more
// .wixobj files and creates a Windows Installer database (.msi or
// .msm). See http://wixtoolset.org/documentation/manual/v3/overview/light.html for options
func (wo *wixOptions) light(ctx context.Context) error {
	args := []string{
		"-nologo",
		"-dcl:high", // compression level
		"-dSourceDir=" + wo.packageRoot,
		"AppFiles.wixobj",
		"Installer.wixobj",
		"-out", "out.msi",
	}

	if wo.skipValidation {
		args = append(args, "-sval")
	}

	_, err := wo.execOut(ctx,
		filepath.Join(wo.wixPath, "light.exe"),
		args...,
	)
	return err

}

func (wo *wixOptions) execOut(ctx context.Context, argv0 string, args ...string) (string, error) {
	logger := ctxlog.FromContext(ctx)

	dockerArgs := []string{
		"run",
		"--entrypoint", "",
		"-v", fmt.Sprintf("%s:%s", wo.packageRoot, wo.packageRoot),
		"-v", fmt.Sprintf("%s:%s", wo.buildDir, wo.buildDir),
		"-w", wo.buildDir,
		wo.dockerImage,
		"wine",
		argv0,
	}

	dockerArgs = append(dockerArgs, args...)

	if wo.dockerImage != "" {
		argv0 = "docker"
		args = dockerArgs
	}

	cmd := wo.execCC(ctx, argv0, args...)

	level.Debug(logger).Log(
		"msg", "execing",
		"cmd", strings.Join(cmd.Args, " "),
	)

	cmd.Dir = wo.buildDir
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "run command %s %v\nstdout=%s\nstderr=%s", argv0, args, stdout, stderr)
	}
	return strings.TrimSpace(stdout.String()), nil
}

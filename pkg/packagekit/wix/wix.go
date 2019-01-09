package wix

import (
	"bytes"
	"context"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

type Wix struct {
	wixPath     string // Where is wix installed
	packageRoot string // What's the root of the packaging files?
	buildDir    string // The wix tools want to work in a build dir.
	msArch      string // What's the microsoft archtecture name?

	execCC func(context.Context, string, ...string) *exec.Cmd // Allows test overrides
}

func New(packageRoot, buildDir string) (*Wix, error) {
	wix := &Wix{
		wixPath:     `C:\wix311`, // FIXME: WTF go, why doesn't filepath.Join work? It produces relative paths
		buildDir:    buildDir,
		packageRoot: packageRoot,

		execCC: exec.CommandContext,
	}

	switch runtime.GOARCH {
	case "386":
		wix.msArch = "x86"
	case "amd64":
		wix.msArch = "x64"
	default:
		return nil, errors.Errorf("unknown arch for windows %s", runtime.GOARCH)
	}

	return wix, nil
}

// InstallWXS takes an io.Reader and drops it's content into the builddir as installer.wxs
func (wix *Wix) InstallWXS(installWXS []byte) error {
	installPath := filepath.Join(wix.buildDir, "Installer.wxs")

	if err := ioutil.WriteFile(
		installPath,
		installWXS,
		0644); err != nil {
		return errors.Wrapf(err, "writing %s", installPath)
	}

	return nil
}

// Heat invokes wix's heat command. This examines a directory and
// "harvests" the files into an xml structure. See
// http://wixtoolset.org/documentation/manual/v3/overview/heat.html
func (wix *Wix) Heat(ctx context.Context) error {
	_, err := wix.execOut(ctx,

		filepath.Join(wix.wixPath, "heat.exe"),
		"dir", wix.packageRoot,
		"-nologo",
		"-gg", "-g1",
		"-srd",
		"-sfrag",
		"-ke",
		"-cg", "AppFiles",
		"-template", "fragment",
		"-dr", "INSTALLDIR",
		"-var", "var.SourceDir",
		"-out", "AppFiles.wxs",
	)
	return err
}

// Candle invokes wix's candle command. This is the wix compiler, It
// preprocesses and compiles WiX source files into object files
// (.wixobj).
func (wix *Wix) Candle(ctx context.Context) error {
	_, err := wix.execOut(ctx,
		filepath.Join(wix.wixPath, "candle.exe"),
		"-nologo",
		"-arch", wix.msArch,
		//"-dGoVersion="+version,
		//fmt.Sprintf("-dWixGoVersion=%v.%v.%v", verMajor, verMinor, verPatch),
		//fmt.Sprintf("-dIsWinXPSupported=%v", wixIsWinXPSupported(version)),
		"-dArch="+runtime.GOARCH,
		"-dSourceDir="+wix.packageRoot,
		"Installer.wxs",
		"AppFiles.wxs",
	)
	return err
}

// Light invokes wix's light command. This links and binds one or more
// .wixobj files and creates a Windows Installer database (.msi or
// .msm)
func (wix *Wix) Light() {

}

func (wix *Wix) execOut(ctx context.Context, argv0 string, args ...string) (string, error) {
	cmd := wix.execCC(ctx, argv0, args...)
	cmd.Dir = wix.buildDir
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "run command %s %v\nstdout=%s\nstderr=%s", argv0, args, stdout, stderr)
	}
	return strings.TrimSpace(stdout.String()), nil
}

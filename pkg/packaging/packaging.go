package packaging

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/packagekit"
	"github.com/pkg/errors"
)

const (
	// Enroll secret should be readable only by root
	secretPerms = 0600
)

type InitFlavor string

const (
	LaunchD InitFlavor = "launchd"
	SystemD            = "systemd"
	Init               = "init"
)

type PlatformFlavor string

const (
	Darwin  PlatformFlavor = "darwin"
	Windows                = "windows"
	Linux                  = "linux"
)

type PackageFlavor string

const (
	Pkg PackageFlavor = "pkg"
	Tar               = "tar"
	Deb               = "deb"
	Rpm               = "rpm"
	Msi               = "msi"
)

type Target struct {
	Init     InitFlavor
	Package  PackageFlavor
	Platform PlatformFlavor
}

func (f *Target) String() string {
	return fmt.Sprintf("%s,%s,%s", f.Platform, f.Init, f.Package)
}

// CreatePackage takes the launcher specific PackageOptions, and a
// target platform, and creates the package. It does this by
// converting the PackageOptions into a set of configuration and
// actions.
//
// TODO "/tmp" is probably wrong on windows
func CreatePackage(po PackageOptions, t Target) error {
	packageRoot, err := ioutil.TempDir("/tmp", fmt.Sprintf("package.packageRoot"))
	if err != nil {
		return errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	// TODO / FIXME
	//defer os.RemoveAll(packageRoot)
	fmt.Printf("hi seph: %s\n", packageRoot)

	scriptRoot, err := ioutil.TempDir("/tmp", fmt.Sprintf("package.packageRoot"))
	if err != nil {
		return errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(scriptRoot)

	binDir, err := binaryDirectory(packageRoot, po.Identifier, t)
	if err != nil {
		return errors.Wrapf(err, "bin dir for %s", t.String())
	}

	confDir, err := configurationDirectory(packageRoot, po.Identifier, t)
	if err != nil {
		return errors.Wrapf(err, "conf dir for %s", t.String())
	}

	rootDir, err := rootDirectory(packageRoot, po.Identifier, po.Hostname, t)
	if err != nil {
		return errors.Wrapf(err, "root dir for %s", t.String())
	}

	launcherEnv := map[string]string{
		"KOLIDE_LAUNCHER_HOSTNAME":           po.Hostname,
		"KOLIDE_LAUNCHER_UPDATE_CHANNEL":     po.UpdateChannel,
		"KOLIDE_LAUNCHER_ROOT_DIRECTORY":     rootDir,
		"KOLIDE_LAUNCHER_OSQUERYD_PATH":      filepath.Join(binDir, "osqueryd"),
		"KOLIDE_LAUNCHER_ENROLL_SECRET_PATH": filepath.Join(confDir, "secret"),
	}

	launcherFlags := []string{}

	if po.InitialRunner {
		launcherFlags = append(launcherFlags, "--with_initial_runner")
	}

	if po.Control && po.ControlHostname != "" {
		launcherEnv["KOLIDE_CONTROL_HOSTNAME"] = po.ControlHostname
	}

	if po.Autoupdate && po.UpdateChannel != "" {
		launcherFlags = append(launcherFlags, "--autoupdate")
		launcherEnv["KOLIDE_LAUNCHER_UPDATE_CHANNEL"] = po.UpdateChannel
	}

	if po.CertPins != "" {
		launcherEnv["KOLIDE_LAUNCHER_CERT_PINS"] = po.CertPins
	}

	if po.DisableControlTLS {
		launcherFlags = append(launcherFlags, "--disable_control_tls")

	}

	if po.InsecureGrpc {
		launcherFlags = append(launcherFlags, "--insecure_grpc")

	}

	if po.Insecure {
		launcherFlags = append(launcherFlags, "--insecure")

	}

	// Unless we're omitting the secret, write it into the package.
	// Note that we _always_ set KOLIDE_LAUNCHER_ENROLL_SECRET_PATH
	if !po.OmitSecret {
		if err := ioutil.WriteFile(
			filepath.Join(packageRoot, confDir, "secret"),
			[]byte(po.Secret),
			secretPerms,
		); err != nil {
			return errors.Wrap(err, "could not write secret string to file for packaging")
		}
	}

	if po.RootPEM != "" {
		rootPemPath := filepath.Join(confDir, "roots.pem")
		launcherEnv["KOLIDE_LAUNCHER_ROOT_PEM"] = rootPemPath

		if err := fs.CopyFile(po.RootPEM, filepath.Join(packageRoot, rootPemPath)); err != nil {
			return errors.Wrap(err, "copy root PEM")
		}

		if err := os.Chmod(filepath.Join(packageRoot, rootPemPath), 0600); err != nil {
			return errors.Wrap(err, "chmod root PEM")
		}
	}

	initOptions := &packagekit.InitOptions{
		Name:        "launcher",
		Description: "The Kolide Launcher",
		Path:        filepath.Join(binDir, "launcher"),
		Identifier:  po.Identifier,
		Flags:       launcherFlags,
		Environment: launcherEnv,
	}

	if err := setupInit(packageRoot, initOptions, t); err != nil {
		return errors.Wrapf(err, "setup init script for %s", t.String())
	}

	// TODO seperate launcher versions, parallization, osquery-extension.ext
	for _, binaryName := range []string{"osqueryd", "launcher"} {
		if err := getBinary(packageRoot, po, t, binDir, binaryName, po.OsqueryVersion); err != nil {
			return errors.Wrapf(err, "fetching binary %s", binaryName)
		}
	}

	if t.Platform == Darwin {
		renderNewSyslogConfig(packageRoot, po, rootDir)
	}

	// TODO: How do we get the extension
	pkgOut, _ := os.Create("/tmp/test.out")

	return makePackage(pkgOut, packageRoot, po, t)
}

func getBinary(packageRoot string, po PackageOptions, t Target, binDir, binaryName, binaryVersion string) error {
	localPath, err := FetchBinary(po.CacheDir, binaryName, binaryVersion, string(t.Platform))
	if err != nil {
		return errors.Wrapf(err, "could not fetch path to binary %s %s", binaryName, binaryVersion)
	}
	if err := fs.CopyFile(
		localPath,
		filepath.Join(packageRoot, binDir, binaryName),
	); err != nil {
		return errors.Wrapf(err, "could not copy binary %s", binaryName)
	}
	return nil
}

func makePackage(w io.Writer, packageRoot string, po PackageOptions, t Target) error {
	packagekitops := &packagekit.PackageOptions{
		Name:    "launcher",
		Version: po.PackageVersion,
		Root:    packageRoot,
	}

	var packageFunc func(io.Writer, *packagekit.PackageOptions, ...packagekit.PkgOption) error

	switch {
	case t.Package == Deb:
		packageFunc = packagekit.PackageDeb
	case t.Package == Rpm:
		packageFunc = packagekit.PackageRPM
	case t.Package == Pkg:
		packageFunc = packagekit.PackagePkg
	default:
		return errors.Errorf("Don't know how to package %s", t.String())
	}

	if err := packageFunc(w, packagekitops); err != nil {
		return errors.Wrapf(err, "packaging, target %s", t.String())
	}

	return nil
}

func setupInit(packageRoot string, initOptions *packagekit.InitOptions, f Target) error {
	var dir string
	var file string
	var renderFunc func(io.Writer, *packagekit.InitOptions) error

	switch {
	case f.Platform == Darwin && f.Init == LaunchD:
		dir = "/Library/LaunchDaemons"
		file = fmt.Sprintf("com.%s.launcher.plist", initOptions.Identifier)
		renderFunc = packagekit.RenderLaunchd
	case f.Platform == Linux && f.Init == SystemD:
		dir = "/etc/systemd/system"
		file = fmt.Sprintf("launcher.%s.service", initOptions.Identifier)
		renderFunc = packagekit.RenderSystemd
	default:
		return errors.Errorf("Unsupported target %s", f.String())
	}

	if err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode); err != nil {
		return errors.Wrapf(err, "mkdir failed, target %s", f.String())
	}

	fh, err := os.Create(filepath.Join(packageRoot, dir, file))
	if err != nil {
		return errors.Wrapf(err, "create filehandle, target %s", f.String())
	}
	defer fh.Close()

	if err := renderFunc(fh, initOptions); err != nil {
		return errors.Wrapf(err, "rendering init file, target %s", f.String())
	}

	return nil
}

type PackageOptions struct {
	PackageVersion       string
	OsqueryVersion       string
	Hostname             string
	Secret               string
	MacPackageSigningKey string
	Insecure             bool
	InsecureGrpc         bool
	Autoupdate           bool
	UpdateChannel        string
	Control              bool
	InitialRunner        bool
	ControlHostname      string
	DisableControlTLS    bool
	Identifier           string
	OmitSecret           bool
	CertPins             string
	RootPEM              string
	OutputPathDir        string
	CacheDir             string
	Systemd              bool
}

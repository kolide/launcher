package packaging

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/packagekit"
	"github.com/pkg/errors"
)

const (
	// Enroll secret should be readable only by root
	secretPerms = 0600
)

// PackageOptions encapsulates the launcher build options. It's
// populated by callers, such as command line flags. It may change.
type PackageOptions struct {
	PackageVersion    string
	OsqueryVersion    string
	LauncherVersion   string
	Hostname          string
	Secret            string
	SigningKey        string
	Insecure          bool
	InsecureGrpc      bool
	Autoupdate        bool
	UpdateChannel     string
	Control           bool
	InitialRunner     bool
	ControlHostname   string
	DisableControlTLS bool
	Identifier        string
	OmitSecret        bool
	CertPins          string
	RootPEM           string
	OutputPathDir     string
	CacheDir          string
}

// Target is the platform being targetted by the build. As "platform"
// has several axis, we use a stuct to convey them.
type Target struct {
	Init     InitFlavor
	Package  PackageFlavor
	Platform PlatformFlavor
}

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

func (f *Target) String() string {
	return fmt.Sprintf("%s,%s,%s", f.Platform, f.Init, f.Package)
}

// CreatePackage takes the launcher specific PackageOptions, and a
// target platform, and creates the package. It does this by
// converting the PackageOptions into a set of configuration and
// actions.
//
// TODO "/tmp" is probably wrong on windows
func CreatePackage(w io.Writer, po PackageOptions, t Target) error {
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

	initFile, err := setupInit(packageRoot, initOptions, t)
	if err != nil {
		return errors.Wrapf(err, "setup init script for %s", t.String())
	}

	postInst, err := setupPostinst(po, initFile, t)
	if err != nil {
		return errors.Wrapf(err, "setup postInst for %s", t.String())
	}

	// Install binaries into packageRoot
	// TODO parallization, osquery-extension.ext
	if err := getBinary(packageRoot, po, t, binDir, "osqueryd", po.OsqueryVersion); err != nil {
		return errors.Wrapf(err, "fetching binary osqueryd")
	}

	if err := getBinary(packageRoot, po, t, binDir, "launcher", po.LauncherVersion); err != nil {
		return errors.Wrapf(err, "fetching binary osqueryd")
	}

	if t.Platform == Darwin {
		renderNewSyslogConfig(packageRoot, po, rootDir)
	}

	packagekitops := &packagekit.PackageOptions{
		Name:       "launcher",
		Postinst:   postInst,
		Prerm:      nil,
		Root:       packageRoot,
		SigningKey: po.SigningKey,
		Version:    po.PackageVersion,
	}

	return makePackage(w, packageRoot, packagekitops, t)
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

func makePackage(w io.Writer, packageRoot string, packagekitops *packagekit.PackageOptions, t Target) error {

	switch {
	case t.Package == Deb:
		if err := packagekit.PackageDeb(w, packagekitops); err != nil {
			return errors.Wrapf(err, "packaging, target %s", t.String())
		}

	case t.Package == Rpm:
		if err := packagekit.PackageRPM(w, packagekitops); err != nil {
			return errors.Wrapf(err, "packaging, target %s", t.String())
		}
	case t.Package == Pkg:
		if err := packagekit.PackagePkg(w, packagekitops); err != nil {
			return errors.Wrapf(err, "packaging, target %s", t.String())
		}
	default:
		return errors.Errorf("Don't know how to package %s", t.String())
	}

	return nil
}

func renderNewSyslogConfig(packageRoot string, po PackageOptions, rootDir string) error {
	// Set logdir, we can assume this is darwin
	logDir := fmt.Sprintf("/var/log/%s", po.Identifier)
	newSysLogDirectory := filepath.Join("/etc", "newsyslog.d")

	if err := os.MkdirAll(filepath.Join(packageRoot, newSysLogDirectory), fs.DirMode); err != nil {
		return errors.Wrap(err, "making newsyslog dir")
	}

	newSysLogPath := filepath.Join(packageRoot, newSysLogDirectory, fmt.Sprintf("%s.conf", po.Identifier))
	newSyslogFile, err := os.Create(newSysLogPath)
	if err != nil {
		return errors.Wrap(err, "creating newsyslog conf file")
	}
	defer newSyslogFile.Close()

	logOptions := struct {
		LogPath string
		PidPath string
	}{
		LogPath: filepath.Join(logDir, "*.log"),
		PidPath: filepath.Join(rootDir, "launcher.pid"),
	}

	syslogTemplate := `# logfilename          [owner:group]    mode count size when  flags [/pid_file] [sig_num]
{{.LogPath}}               640  3  4000   *   G  {{.PidPath}} 15`
	tmpl, err := template.New("syslog").Parse(syslogTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse postinstall template")
	}
	if err := tmpl.ExecuteTemplate(newSyslogFile, "syslog", logOptions); err != nil {
		return errors.Wrap(err, "execute template")
	}
	return nil
}

func setupInit(packageRoot string, initOptions *packagekit.InitOptions, f Target) (string, error) {
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
		return "", errors.Errorf("Unsupported target %s", f.String())
	}

	if err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode); err != nil {
		return "", errors.Wrapf(err, "mkdir failed, target %s", f.String())
	}

	fh, err := os.Create(filepath.Join(packageRoot, dir, file))
	if err != nil {
		return "", errors.Wrapf(err, "create filehandle, target %s", f.String())
	}
	defer fh.Close()

	if err := renderFunc(fh, initOptions); err != nil {
		return "", errors.Wrapf(err, "rendering init file, target %s", f.String())
	}

	return filepath.Join(dir, file), nil
}

func setupPrerm(po PackageOptions, initFile string, t Target) (io.Reader, error) {
	switch {
	case t.Platform == Darwin && t.Init == LaunchD:
	case t.Platform == Linux && t.Init == SystemD:
	case t.Platform == Linux && t.Init == Init:
		// TODO double check if this is init, or what
	}

	// If we don't match in the case statement, log that we're ignoring
	// the setup, and move on. Don't throw an error. FIXME: Setup
	// logging
	return nil, nil
}

// TODO these names are wrong for linux
func setupPostinst(po PackageOptions, initFile string, t Target) (io.Reader, error) {
	switch {
	case t.Platform == Darwin && t.Init == LaunchD:
		var output bytes.Buffer
		if err := renderLaunchdPostinst(&output, po.Identifier, initFile); err != nil {
			return nil, errors.Wrap(err, "setupPostinst")
		}
		return &output, nil

	case t.Platform == Linux && t.Init == SystemD:
		contents := `#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable launcher
systemctl restart launcher`
		return strings.NewReader(contents), nil

	case t.Platform == Linux && t.Init == Init:
		// TODO double check if this is init, or what
		contents := `#!/bin/bash
sudo service launcher restart`
		return strings.NewReader(contents), nil
	}

	// If we don't match in the case statement, log that we're ignoring
	// the setup, and move on. Don't throw an error. FIXME: Setup
	// logging
	return nil, nil

}

func renderLaunchdPostinst(w io.Writer, identifier, initFile string) error {
	var data = struct {
		Identifier string
		Path       string
	}{
		Identifier: fmt.Sprintf("com.%s.launcher", identifier),
		Path:       initFile,
	}

	postinstallTemplate := `#!/bin/bash

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop {{.Identifier}}

sleep 5

/bin/launchctl unload {{.Path}}
/bin/launchctl load {{.Path}}`

	t, err := template.New("postinstall").Parse(postinstallTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse postinstall template")
	}
	return t.ExecuteTemplate(w, "postinstall", data)
}

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

	target        Target                     // Target build platform
	initOptions   *packagekit.InitOptions    // options we'll pass to the packagekit renderers
	packagekitops *packagekit.PackageOptions // options for packagekit packagers

	// These are build machine local directories. They are absolute path.
	packageRoot string // temp directory that will become the package
	scriptRoot  string // temp directory to hold scripts. Many packaging systems treat these as metadata.

	// These are paths _internal_ to the package
	rootDir  string // launcher's root directory
	binDir   string // where to place binaries (eg: /usr/local/bin)
	confDir  string // where to place configs (eg: /etc/<name>)
	initFile string // init file, the path is used in the various scripts.

}

// internalOptions is a struct to hold a bunch of internally generated settings. It's here to simplify passing arguments between our packaging routines.
type internalOptions struct {
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

// NewPackager returns a PackageOptions struct. You can, just create one directly.
func NewPackager() *PackageOptions {
	return &PackageOptions{}
}

func (p *PackageOptions) Prepare(target Target) error {

	p.target = target

	var err error

	// TODO "/tmp" is probably wrong on windows
	if p.packageRoot, err = ioutil.TempDir("/tmp", fmt.Sprintf("package.packageRoot")); err != nil {
		return errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	//TODO defer os.RemoveAll(p.packageRoot)
	fmt.Printf("hi seph: %s\n", p.packageRoot)

	// TODO "/tmp" is probably wrong on windows
	if p.scriptRoot, err = ioutil.TempDir("/tmp", fmt.Sprintf("package.scriptRoot")); err != nil {
		return errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	// TODO defer os.RemoveAll(intOpts.ScriptRoot)

	if err := p.setupDirectories(); err != nil {
		return errors.Wrap(err, "setup directories")
	}

	launcherEnv := map[string]string{
		"KOLIDE_LAUNCHER_HOSTNAME":           p.Hostname,
		"KOLIDE_LAUNCHER_UPDATE_CHANNEL":     p.UpdateChannel,
		"KOLIDE_LAUNCHER_ROOT_DIRECTORY":     p.rootDir,
		"KOLIDE_LAUNCHER_OSQUERYD_PATH":      filepath.Join(p.binDir, "osqueryd"),
		"KOLIDE_LAUNCHER_ENROLL_SECRET_PATH": filepath.Join(p.confDir, "secret"),
	}

	launcherFlags := []string{}

	if p.InitialRunner {
		launcherFlags = append(launcherFlags, "--with_initial_runner")
	}

	if p.Control && p.ControlHostname != "" {
		launcherEnv["KOLIDE_CONTROL_HOSTNAME"] = p.ControlHostname
	}

	if p.Autoupdate && p.UpdateChannel != "" {
		launcherFlags = append(launcherFlags, "--autoupdate")
		launcherEnv["KOLIDE_LAUNCHER_UPDATE_CHANNEL"] = p.UpdateChannel
	}

	if p.CertPins != "" {
		launcherEnv["KOLIDE_LAUNCHER_CERT_PINS"] = p.CertPins
	}

	if p.DisableControlTLS {
		launcherFlags = append(launcherFlags, "--disable_control_tls")

	}

	if p.InsecureGrpc {
		launcherFlags = append(launcherFlags, "--insecure_grpc")

	}

	if p.Insecure {
		launcherFlags = append(launcherFlags, "--insecure")

	}

	// Unless we're omitting the secret, write it into the package.
	// Note that we _always_ set KOLIDE_LAUNCHER_ENROLL_SECRET_PATH
	if !p.OmitSecret {
		if err := ioutil.WriteFile(
			filepath.Join(p.packageRoot, p.confDir, "secret"),
			[]byte(p.Secret),
			secretPerms,
		); err != nil {
			return errors.Wrap(err, "could not write secret string to file for packaging")
		}
	}

	if p.RootPEM != "" {
		rootPemPath := filepath.Join(p.confDir, "roots.pem")
		launcherEnv["KOLIDE_LAUNCHER_ROOT_PEM"] = rootPemPath

		if err := fs.CopyFile(p.RootPEM, filepath.Join(p.packageRoot, rootPemPath)); err != nil {
			return errors.Wrap(err, "copy root PEM")
		}

		if err := os.Chmod(filepath.Join(p.packageRoot, rootPemPath), 0600); err != nil {
			return errors.Wrap(err, "chmod root PEM")
		}
	}

	p.initOptions = &packagekit.InitOptions{
		Name:        "launcher",
		Description: "The Kolide Launcher",
		Path:        filepath.Join(p.binDir, "launcher"),
		Identifier:  p.Identifier,
		Flags:       launcherFlags,
		Environment: launcherEnv,
	}

	p.packagekitops = &packagekit.PackageOptions{
		Name:       "launcher",
		Postinst:   nil,
		Prerm:      nil,
		Root:       p.packageRoot,
		SigningKey: p.SigningKey,
		Version:    p.PackageVersion,
	}

	if err := p.setupInit(); err != nil {
		return errors.Wrapf(err, "setup init script for %s", p.target.String())
	}

	if err := p.setupPostinst(); err != nil {
		return errors.Wrapf(err, "setup postInst for %s", p.target.String())
	}

	if err := p.setupPrerm(); err != nil {
		return errors.Wrapf(err, "setup setupPrerm for %s", p.target.String())
	}

	// Install binaries into packageRoot
	// TODO parallization, osquery-extension.ext
	if err := p.getBinary("osqueryd", p.OsqueryVersion); err != nil {
		return errors.Wrapf(err, "fetching binary osqueryd")
	}

	if err := p.getBinary("launcher", p.LauncherVersion); err != nil {
		return errors.Wrapf(err, "fetching binary osqueryd")
	}

	if p.target.Platform == Darwin {
		if err := p.renderNewSyslogConfig(); err != nil {
			return errors.Wrap(err, "render")
		}
	}

	//return makePackage(w, packageRoot, packagekitops, t)
	return nil
}

func (p *PackageOptions) getBinary(binaryName, binaryVersion string) error {
	localPath, err := FetchBinary(p.CacheDir, binaryName, binaryVersion, string(p.target.Platform))
	if err != nil {
		return errors.Wrapf(err, "could not fetch path to binary %s %s", binaryName, binaryVersion)
	}
	if err := fs.CopyFile(
		localPath,
		filepath.Join(p.packageRoot, p.binDir, binaryName),
	); err != nil {
		return errors.Wrapf(err, "could not copy binary %s", binaryName)
	}
	return nil
}

func (p *PackageOptions) Build(w io.Writer) error {

	switch {
	case p.target.Package == Deb:
		if err := packagekit.PackageDeb(w, p.packagekitops); err != nil {
			return errors.Wrapf(err, "packaging, target %s", p.target.String())
		}

	case p.target.Package == Rpm:
		if err := packagekit.PackageRPM(w, p.packagekitops); err != nil {
			return errors.Wrapf(err, "packaging, target %s", p.target.String())
		}
	case p.target.Package == Pkg:
		if err := packagekit.PackagePkg(w, p.packagekitops); err != nil {
			return errors.Wrapf(err, "packaging, target %s", p.target.String())
		}
	default:
		return errors.Errorf("Don't know how to package %s", p.target.String())
	}

	return nil
}

func (p *PackageOptions) renderNewSyslogConfig() error {
	// Set logdir, we can assume this is darwin
	logDir := fmt.Sprintf("/var/log/%s", p.Identifier)
	newSysLogDirectory := filepath.Join("/etc", "newsyslog.d")

	if err := os.MkdirAll(filepath.Join(p.packageRoot, newSysLogDirectory), fs.DirMode); err != nil {
		return errors.Wrap(err, "making newsyslog dir")
	}

	newSysLogPath := filepath.Join(p.packageRoot, newSysLogDirectory, fmt.Sprintf("%s.conf", p.Identifier))
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
		PidPath: filepath.Join(p.rootDir, "launcher.pid"),
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

func (p *PackageOptions) setupInit() error {
	var dir string
	var file string
	var renderFunc func(io.Writer, *packagekit.InitOptions) error

	switch {
	case p.target.Platform == Darwin && p.target.Init == LaunchD:
		dir = "/Library/LaunchDaemons"
		file = fmt.Sprintf("com.%s.launcher.plist", p.initOptions.Identifier)
		renderFunc = packagekit.RenderLaunchd
	case p.target.Platform == Linux && p.target.Init == SystemD:
		dir = "/etc/systemd/system"
		file = fmt.Sprintf("launcher.%s.service", p.initOptions.Identifier)
		renderFunc = packagekit.RenderSystemd
	default:
		return errors.Errorf("Unsupported target %s", p.target.String())
	}

	p.initFile = filepath.Join(dir, file)

	if err := os.MkdirAll(filepath.Join(p.packageRoot, dir), fs.DirMode); err != nil {
		return errors.Wrapf(err, "mkdir failed, target %s", p.target.String())
	}

	fh, err := os.Create(filepath.Join(p.packageRoot, p.initFile))
	if err != nil {
		return errors.Wrapf(err, "create filehandle, target %s", p.target.String())
	}
	defer fh.Close()

	if err := renderFunc(fh, p.initOptions); err != nil {
		return errors.Wrapf(err, "rendering init file (%s), target %s", p.target.String())
	}

	return nil
}

func (p *PackageOptions) setupPrerm() error {
	switch {
	case p.target.Platform == Darwin && p.target.Init == LaunchD:
	case p.target.Platform == Linux && p.target.Init == SystemD:
	case p.target.Platform == Linux && p.target.Init == Init:
		// TODO double check if this is init, or what
	}

	// If we don't match in the case statement, log that we're ignoring
	// the setup, and move on. Don't throw an error. FIXME: Setup
	// logging
	return nil
}

// TODO this is all wrong -- these should be templated based on indentifier
func (p *PackageOptions) setupPostinst() error {
	switch {
	case p.target.Platform == Darwin && p.target.Init == LaunchD:
		var output bytes.Buffer
		if err := renderLaunchdPostinst(&output, p.Identifier, p.initFile); err != nil {
			return errors.Wrap(err, "setupPostinst")
		}
		p.packagekitops.Postinst = &output

	case p.target.Platform == Linux && p.target.Init == SystemD:
		contents := `#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable launcher
systemctl restart launcher`
		p.packagekitops.Postinst = strings.NewReader(contents)

	case p.target.Platform == Linux && p.target.Init == Init:
		// TODO double check if this is init, or what
		contents := `#!/bin/bash
sudo service launcher restart`
		p.packagekitops.Postinst = strings.NewReader(contents)
	}

	// If we don't match in the case statement, log that we're ignoring
	// the setup, and move on. Don't throw an error. FIXME: Setup
	// logging
	return nil

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

func (p *PackageOptions) setupDirectories() error {
	switch p.target.Platform {
	case Linux, Darwin:
		p.binDir = filepath.Join("/usr/local", p.Identifier, "bin")
		p.confDir = filepath.Join("/etc", p.Identifier)
		p.rootDir = filepath.Join("/var", p.Identifier, sanitizeHostname(p.Hostname))

	default:
		return errors.Errorf("Unknown platform %s", string(p.target.Platform))
	}

	for _, d := range []string{p.binDir, p.confDir, p.rootDir} {
		if err := os.MkdirAll(filepath.Join(p.packageRoot, d), fs.DirMode); err != nil {
			return errors.Wrapf(err, "create dir (%s) for %s", d, p.target.String())
		}
	}
	return nil
}

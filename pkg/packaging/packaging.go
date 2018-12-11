package packaging

import (
	"context"
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
	"go.opencensus.io/trace"
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
	ExtensionVersion  string
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
	CacheDir          string

	target        Target                     // Target build platform
	initOptions   *packagekit.InitOptions    // options we'll pass to the packagekit renderers
	packagekitops *packagekit.PackageOptions // options for packagekit packagers
	packageWriter io.Writer                  // Where to write the file

	// These are build machine local directories. They are absolute paths.
	packageRoot string // temp directory that will become the package
	scriptRoot  string // temp directory to hold scripts. Many packaging systems treat these as metadata.

	// These are paths _internal_ to the package
	rootDir  string // launcher's root directory
	binDir   string // where to place binaries (eg: /usr/local/bin)
	confDir  string // where to place configs (eg: /etc/<name>)
	initFile string // init file, the path is used in the various scripts.

}

// NewPackager returns a PackageOptions struct. You can, just create one directly.
func NewPackager() *PackageOptions {
	return &PackageOptions{}
}

func (p *PackageOptions) Build(ctx context.Context, packageWriter io.Writer, target Target) error {

	p.target = target
	p.packageWriter = packageWriter

	var err error

	if p.packageRoot, err = ioutil.TempDir("", "package.packageRoot"); err != nil {
		return errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(p.packageRoot)

	if p.scriptRoot, err = ioutil.TempDir("", fmt.Sprintf("package.scriptRoot")); err != nil {
		return errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(p.scriptRoot)

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
		Identifier: p.Identifier,
		Root:       p.packageRoot,
		Scripts:    p.scriptRoot,
		SigningKey: p.SigningKey,
		Version:    p.PackageVersion,
	}

	if err := p.setupInit(ctx); err != nil {
		return errors.Wrapf(err, "setup init script for %s", p.target.String())
	}

	if err := p.setupPostinst(ctx); err != nil {
		return errors.Wrapf(err, "setup postInst for %s", p.target.String())
	}

	if err := p.setupPrerm(ctx); err != nil {
		return errors.Wrapf(err, "setup setupPrerm for %s", p.target.String())
	}

	// Install binaries into packageRoot
	// TODO parallization, osquery-extension.ext
	// TODO windows file extensions
	if err := p.getBinary(ctx, p.target.PlatformBinaryName("osqueryd"), p.OsqueryVersion); err != nil {
		return errors.Wrapf(err, "fetching binary osqueryd")
	}

	if err := p.getBinary(ctx, p.target.PlatformBinaryName("launcher"), p.LauncherVersion); err != nil {
		return errors.Wrapf(err, "fetching binary launcher")
	}

	if err := p.getBinary(ctx, p.target.PlatformExtensionName("osquery-extension"), p.ExtensionVersion); err != nil {
		return errors.Wrapf(err, "fetching binary launcher")
	}

	if p.target.Platform == Darwin {
		if err := p.renderNewSyslogConfig(ctx); err != nil {
			return errors.Wrap(err, "render")
		}
	}

	if err := p.makePackage(ctx); err != nil {
		return errors.Wrap(err, "making package")
	}

	return nil
}

// getBinary will fetch binaries from places and copy them into our
// package root. The default case is to assume binaryVersion is a
// string, and to download from TUF. But it it starts with a character
// that looks like a file path, treat is as something on the
// filesystem.
//
// TODO: add in file:// URLs
func (p *PackageOptions) getBinary(ctx context.Context, binaryName, binaryVersion string) error {
	ctx, span := trace.StartSpan(ctx, fmt.Sprintf("packaging.getBinary.%s", binaryName))
	defer span.End()

	var err error
	var localPath string

	switch {
	case strings.HasPrefix(binaryVersion, "./"), strings.HasPrefix(binaryVersion, "/"):
		localPath = binaryVersion
	default:
		localPath, err = FetchBinary(ctx, p.CacheDir, binaryName, binaryVersion, string(p.target.Platform))
		if err != nil {
			return errors.Wrapf(err, "could not fetch path to binary %s %s", binaryName, binaryVersion)
		}
	}

	if err := fs.CopyFile(
		localPath,
		filepath.Join(p.packageRoot, p.binDir, binaryName),
	); err != nil {
		return errors.Wrapf(err, "could not copy binary %s", binaryName)
	}
	return nil
}

func (p *PackageOptions) makePackage(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "packaging.makePackage")
	defer span.End()

	switch {
	case p.target.Package == Deb:
		if err := packagekit.PackageFPM(ctx, p.packageWriter, p.packagekitops, packagekit.AsDeb()); err != nil {
			return errors.Wrapf(err, "packaging, target %s", p.target.String())
		}

	case p.target.Package == Rpm:
		if err := packagekit.PackageFPM(ctx, p.packageWriter, p.packagekitops, packagekit.AsRPM()); err != nil {
			return errors.Wrapf(err, "packaging, target %s", p.target.String())
		}
	case p.target.Package == Pkg:
		if err := packagekit.PackagePkg(ctx, p.packageWriter, p.packagekitops); err != nil {
			return errors.Wrapf(err, "packaging, target %s", p.target.String())
		}
	default:
		return errors.Errorf("Don't know how to package %s", p.target.String())
	}

	return nil
}

func (p *PackageOptions) renderNewSyslogConfig(ctx context.Context) error {
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
		return errors.Wrap(err, "not able to parse newsyslog template")
	}
	if err := tmpl.ExecuteTemplate(newSyslogFile, "syslog", logOptions); err != nil {
		return errors.Wrap(err, "execute template")
	}
	return nil
}

func (p *PackageOptions) setupInit(ctx context.Context) error {
	var dir string
	var file string
	var renderFunc func(context.Context, io.Writer, *packagekit.InitOptions) error

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

	if err := renderFunc(ctx, fh, p.initOptions); err != nil {
		return errors.Wrapf(err, "rendering init file (%s), target %s", p.initFile, p.target.String())
	}

	return nil
}

func (p *PackageOptions) setupPrerm(ctx context.Context) error {
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

func (p *PackageOptions) setupPostinst(ctx context.Context) error {
	var postinstTemplate string
	identifier := p.Identifier

	switch {
	case p.target.Platform == Darwin && p.target.Init == LaunchD:
		postinstTemplate = postinstallLauncherTemplate()
		identifier = fmt.Sprintf("com.%s.launcher", p.Identifier)
	case p.target.Platform == Linux && p.target.Init == SystemD:
		postinstTemplate = postinstallSystemdTemplate()
	case p.target.Platform == Linux && p.target.Init == Init:
		postinstTemplate = postinstallInitTemplate()
	default:
		// If we don't match in the case statement, log that we're ignoring
		// the setup, and move on. Don't throw an error.
		// logging
		return nil
	}

	var data = struct {
		Identifier string
		Path       string
	}{
		Identifier: identifier,
		Path:       p.initFile,
	}

	t, err := template.New("postinstall").Parse(postinstTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse template")
	}

	fh, err := os.Create(filepath.Join(p.scriptRoot, "postinstall"))
	if err != nil {
		return errors.Wrapf(err, "create postinstall filehandle")
	}
	defer fh.Close()

	if err := t.ExecuteTemplate(fh, "postinstall", data); err != nil {
		return errors.Wrap(err, "executing template")
	}

	return nil
}

func postinstallInitTemplate() string {
	return `#!/bin/sh
sudo service launcher.{{.Identifier}} restart`
}

func postinstallLauncherTemplate() string {
	return `#!/bin/bash

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop {{.Identifier}}

sleep 5

/bin/launchctl unload {{.Path}}
/bin/launchctl load {{.Path}}`
}

func postinstallSystemdTemplate() string {
	return `#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable launcher.{{.Identifier}}
systemctl restart launcher.{{.Identifier}}`
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

package packaging

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/packagekit"

	"go.opencensus.io/trace"
)

//go:embed assets/*
var assets embed.FS

const (
	// Enroll secret should be readable only by root
	secretPerms = 0600
)

// PackageOptions encapsulates the launcher build options. It's
// populated by callers, such as command line flags. It may change.
type PackageOptions struct {
	PackageVersion    string // What version in this package. If unset, autodetection will be attempted.
	OsqueryVersion    string
	OsqueryFlags      []string // Additional flags to pass to the runtime osquery instance
	LauncherVersion   string
	ExtensionVersion  string
	Hostname          string
	Secret            string
	Transport         string
	Insecure          bool
	InsecureTransport bool
	UpdateChannel     string
	InitialRunner     bool
	DisableControlTLS bool
	Identifier        string
	Title             string
	OmitSecret        bool
	CertPins          string
	RootPEM           string
	CacheDir          string
	NotaryURL         string
	MirrorURL         string
	NotaryPrefix      string
	WixPath           string
	MSIUI             bool
	WixSkipCleanup    bool
	DisableService    bool

	// Normally we'd download the same version we bake into the
	// autoupdate. But occasionally, it's handy to make a package
	// with a different version.
	LauncherDownloadVersionOverride string
	OsqueryDownloadVersionOverride  string

	AppleNotarizeAccountId   string   // The 10 character apple account id
	AppleNotarizeAppPassword string   // app password for notarization service
	AppleNotarizeUserId      string   // User id to authenticate to the notarization service with
	AppleSigningKey          string   // apple signing key
	WindowsSigntoolArgs      []string // Extra args for signtool. May be needed for finding a key
	WindowsUseSigntool       bool     // whether to use signtool.exe on windows

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

	execCC func(context.Context, string, ...string) *exec.Cmd
}

// NewPackager returns a PackageOptions struct. You can, however, just
// create one directly.
func NewPackager() *PackageOptions {
	return &PackageOptions{}
}

func (p *PackageOptions) Build(ctx context.Context, packageWriter io.Writer, target Target) error {

	p.target = target
	p.packageWriter = packageWriter

	var err error

	if p.packageRoot, err = os.MkdirTemp("", "package.packageRoot"); err != nil {
		return fmt.Errorf("unable to create temporary packaging root directory: %w", err)
	}
	defer os.RemoveAll(p.packageRoot)

	if p.scriptRoot, err = os.MkdirTemp("", fmt.Sprintf("package.scriptRoot")); err != nil {
		return fmt.Errorf("unable to create temporary packaging root directory: %w", err)
	}
	defer os.RemoveAll(p.scriptRoot)

	if err := p.setupDirectories(); err != nil {
		return fmt.Errorf("setup directories: %w", err)
	}

	flagFilePath := filepath.Join(p.confDir, "launcher.flags")
	flagFile, err := os.Create(filepath.Join(p.packageRoot, flagFilePath))
	if err != nil {
		return fmt.Errorf("creating flag file: %w", err)
	}
	defer flagFile.Close()

	launcherMapFlags := map[string]string{
		"hostname":           p.Hostname,
		"root_directory":     p.canonicalizePath(p.rootDir),
		"osqueryd_path":      p.canonicalizePath(filepath.Join(p.binDir, "osqueryd")),
		"enroll_secret_path": p.canonicalizePath(filepath.Join(p.confDir, "secret")),
	}

	launcherBoolFlags := []string{}

	if p.InitialRunner {
		launcherBoolFlags = append(launcherBoolFlags, "with_initial_runner")
	}

	if p.UpdateChannel != "" {
		launcherBoolFlags = append(launcherBoolFlags, "autoupdate")
		launcherMapFlags["update_channel"] = p.UpdateChannel
	}

	if p.CertPins != "" {
		launcherMapFlags["cert_pins"] = p.CertPins
	}

	if p.DisableControlTLS {
		launcherBoolFlags = append(launcherBoolFlags, "disable_control_tls")
	}

	if p.Transport != "" {
		launcherMapFlags["transport"] = p.Transport
	}

	if p.InsecureTransport {
		launcherBoolFlags = append(launcherBoolFlags, "insecure_transport")
	}

	if p.Insecure {
		launcherBoolFlags = append(launcherBoolFlags, "insecure")
	}

	if p.NotaryURL != "" {
		launcherMapFlags["notary_url"] = p.NotaryURL
	}

	if p.MirrorURL != "" {
		launcherMapFlags["mirror_url"] = p.MirrorURL
	}

	if p.NotaryPrefix != "" {
		launcherMapFlags["notary_prefix"] = p.NotaryPrefix
	}

	if p.RootPEM != "" {
		rootPemPath := filepath.Join(p.confDir, "roots.pem")
		launcherMapFlags["root_pem"] = p.canonicalizePath(rootPemPath)

		if err := fsutil.CopyFile(p.RootPEM, filepath.Join(p.packageRoot, rootPemPath)); err != nil {
			return fmt.Errorf("copy root PEM: %w", err)
		}

		if err := os.Chmod(filepath.Join(p.packageRoot, rootPemPath), 0600); err != nil {
			return fmt.Errorf("chmod root PEM: %w", err)
		}
	}

	// Write the flags to the flagFile
	for _, k := range launcherBoolFlags {
		if _, err := flagFile.WriteString(fmt.Sprintf("%s\n", k)); err != nil {
			return fmt.Errorf("failed to write %s to flagfile: %w", k, err)
		}
	}
	for k, v := range launcherMapFlags {
		if _, err := flagFile.WriteString(fmt.Sprintf("%s %s\n", k, v)); err != nil {
			return fmt.Errorf("failed to write %s to flagfile: %w", k, err)
		}
	}
	for _, flag := range p.OsqueryFlags {
		if _, err := flagFile.WriteString(fmt.Sprintf("osquery_flag %s\n", flag)); err != nil {
			return fmt.Errorf("failed to write osquery_flag to flagfile: %s: %w", flag, err)
		}

	}

	// Wixtoolset seems to get unhappy if the flagFile is open, and since
	// we're done writing it, may as well close it.
	flagFile.Close()

	// Unless we're omitting the secret, write it into the package.
	// Note that we _always_ set KOLIDE_LAUNCHER_ENROLL_SECRET_PATH
	if !p.OmitSecret {
		if err := os.WriteFile(
			filepath.Join(p.packageRoot, p.confDir, "secret"),
			[]byte(p.Secret),
			secretPerms,
		); err != nil {
			return fmt.Errorf("could not write secret string to file for packaging: %w", err)
		}
	}

	// Install binaries into packageRoot
	// TODO parallization
	// TODO windows file extensions

	if p.OsqueryDownloadVersionOverride == "" {
		p.OsqueryDownloadVersionOverride = p.OsqueryVersion
	}
	if err := p.getBinary(ctx, "osqueryd", p.target.PlatformBinaryName("osqueryd"), p.OsqueryDownloadVersionOverride); err != nil {
		return fmt.Errorf("fetching binary osqueryd: %w", err)
	}

	if p.LauncherDownloadVersionOverride == "" {
		p.LauncherDownloadVersionOverride = p.LauncherVersion
	}

	if err := p.getBinary(ctx, "launcher", p.target.PlatformBinaryName("launcher"), p.LauncherDownloadVersionOverride); err != nil {
		return fmt.Errorf("fetching binary launcher: %w", err)
	}

	// Some darwin specific bits
	if p.target.Platform == Darwin {
		if err := p.renderNewSyslogConfig(ctx); err != nil {
			return fmt.Errorf("render: %w", err)
		}

		// launchd seems to need the log directory to be pre-created. So,
		// we'll do this here. This is a bit ugly, since we're duplicate
		// the directory expansion we do in packagekit.RenderLaunchd. TODO
		// merge logic, and pass the logs paths as an option.
		logDir := filepath.Join(p.packageRoot, "var", "log", p.Identifier)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("mkdir logdir %s: %w", logDir, err)
		}
	}

	// amazon linux ami uses an upstart so old, it doesn't have
	// integrated logging. So we'll need a logrotate config.
	if p.target.Init == UpstartAmazonAMI {
		if err := p.renderLogrotateConfig(ctx); err != nil {
			return fmt.Errorf("render: %w", err)
		}
	}

	// The version string is the version of _launcher_ which we don't
	// know until we've downloaded it.
	if p.PackageVersion == "" {
		if err := p.detectLauncherVersion(ctx); err != nil {
			return fmt.Errorf("version detection: %w", err)
		}
	}

	p.initOptions = &packagekit.InitOptions{
		Name:        "launcher",
		Description: "The Kolide Launcher",
		Path:        p.target.PlatformLauncherPath(p.binDir),
		Identifier:  p.Identifier,
		Flags:       []string{"-config", flagFilePath},
		Environment: map[string]string{},
	}

	if err := p.setupInit(ctx); err != nil {
		return fmt.Errorf("setup init script for %s: %w", p.target.String(), err)
	}

	if err := p.setupPostinst(ctx); err != nil {
		return fmt.Errorf("setup postInst for %s: %w", p.target.String(), err)
	}

	if err := p.setupPrerm(ctx); err != nil {
		return fmt.Errorf("setup setupPrerm for %s: %w", p.target.String(), err)
	}

	if p.Title == "" {
		p.Title = fmt.Sprintf("Launcher agent for %s", p.Identifier)
	}

	p.packagekitops = &packagekit.PackageOptions{
		Name:                     "launcher",
		Identifier:               p.Identifier,
		Title:                    p.Title,
		Root:                     p.packageRoot,
		Scripts:                  p.scriptRoot,
		AppleNotarizeAccountId:   p.AppleNotarizeAccountId,
		AppleNotarizeAppPassword: p.AppleNotarizeAppPassword,
		AppleNotarizeUserId:      p.AppleNotarizeUserId,
		AppleSigningKey:          p.AppleSigningKey,
		WindowsUseSigntool:       p.WindowsUseSigntool,
		WindowsSigntoolArgs:      p.WindowsSigntoolArgs,
		Version:                  p.PackageVersion,
		FlagFile:                 p.canonicalizePath(flagFilePath),
		WixPath:                  p.WixPath,
		WixUI:                    p.MSIUI,
		WixSkipCleanup:           p.WixSkipCleanup,
		DisableService:           p.DisableService,
	}

	if err := p.makePackage(ctx); err != nil {
		return fmt.Errorf("making package: %w", err)
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
func (p *PackageOptions) getBinary(ctx context.Context, symbolicName, binaryName, binaryVersion string) error {
	ctx, span := trace.StartSpan(ctx, fmt.Sprintf("packaging.getBinary.%s", symbolicName))
	defer span.End()

	var err error
	var localPath string

	switch {
	case strings.HasPrefix(binaryVersion, "./"), strings.HasPrefix(binaryVersion, "/"), strings.HasPrefix(binaryVersion, `\`),
		strings.HasPrefix(binaryVersion, "C:"), strings.HasPrefix(binaryVersion, "D:"):
		localPath = binaryVersion
	default:
		localPath, err = FetchBinary(ctx, p.CacheDir, symbolicName, binaryName, binaryVersion, p.target)
		if err != nil {
			return fmt.Errorf("could not fetch path to binary %s %s: %w", binaryName, binaryVersion, err)
		}
	}

	// Check to see if we fetched an app bundle. If so, copy over the app bundle directory.
	appBundlePath := filepath.Join(filepath.Dir(localPath), "Kolide.app")
	appBundleInfo, err := os.Stat(appBundlePath)
	if err == nil && appBundleInfo.IsDir() {
		if err := fsutil.CopyDir(
			appBundlePath,
			filepath.Join(p.packageRoot, p.binDir, "Kolide.app"),
		); err != nil {
			return fmt.Errorf("could not copy app bundle: %w", err)
		}

		return nil
	}

	// Not an app bundle -- just copy the binary.
	if err := fsutil.CopyFile(
		localPath,
		filepath.Join(p.packageRoot, p.binDir, binaryName),
	); err != nil {
		return fmt.Errorf("could not copy binary %s: %w", binaryName, err)
	}
	return nil
}

func (p *PackageOptions) makePackage(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "packaging.makePackage")
	defer span.End()

	// Linux packages used to be distributed named "launcher". We've
	// moved to naming them "launcher-<identifier>". To provide a
	// cleaner package replacement, we can flag this to the underlying
	// packaging systems.
	oldPackageNames := []string{"launcher"}

	switch {
	case p.target.Package == Deb:
		if err := packagekit.PackageFPM(ctx, p.packageWriter, p.packagekitops, packagekit.AsDeb(), packagekit.WithReplaces(oldPackageNames)); err != nil {
			return fmt.Errorf("packaging, target %s: %w", p.target.String(), err)
		}
	case p.target.Package == Rpm:
		if err := packagekit.PackageFPM(ctx, p.packageWriter, p.packagekitops, packagekit.AsRPM(), packagekit.WithReplaces(oldPackageNames)); err != nil {
			return fmt.Errorf("packaging, target %s: %w", p.target.String(), err)
		}

	case p.target.Package == Tar:
		if err := packagekit.PackageFPM(ctx, p.packageWriter, p.packagekitops, packagekit.AsTar(), packagekit.WithReplaces(oldPackageNames)); err != nil {
			return fmt.Errorf("packaging, target %s: %w", p.target.String(), err)
		}
	case p.target.Package == Pacman:
		if err := packagekit.PackageFPM(ctx, p.packageWriter, p.packagekitops, packagekit.AsPacman(), packagekit.WithReplaces(oldPackageNames)); err != nil {
			return fmt.Errorf("packaging, target %s: %w", p.target.String(), err)
		}
	case p.target.Package == Pkg:
		if err := packagekit.PackagePkg(ctx, p.packageWriter, p.packagekitops); err != nil {
			return fmt.Errorf("packaging, target %s: %w", p.target.String(), err)
		}
	case p.target.Package == Msi:
		// pass whether to include a service as a bool argument to PackageWixMSI
		includeService := p.target.Init == WindowsService
		if err := packagekit.PackageWixMSI(ctx, p.packageWriter, p.packagekitops, includeService); err != nil {
			return fmt.Errorf("packaging, target %s: %w", p.target.String(), err)
		}
	default:
		return fmt.Errorf("Don't know how to package %s", p.target.String())
	}

	return nil
}

func (p *PackageOptions) renderNewSyslogConfig(ctx context.Context) error {
	// Set logdir, we can assume this is darwin
	logDir := fmt.Sprintf("/var/log/%s", p.Identifier)
	newSysLogDirectory := filepath.Join("/etc", "newsyslog.d")

	if err := os.MkdirAll(filepath.Join(p.packageRoot, newSysLogDirectory), fsutil.DirMode); err != nil {
		return fmt.Errorf("making newsyslog dir: %w", err)
	}

	newSysLogPath := filepath.Join(p.packageRoot, newSysLogDirectory, fmt.Sprintf("%s.conf", p.Identifier))
	newSyslogFile, err := os.Create(newSysLogPath)
	if err != nil {
		return fmt.Errorf("creating newsyslog conf file: %w", err)
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
		return fmt.Errorf("not able to parse newsyslog template: %w", err)
	}
	if err := tmpl.ExecuteTemplate(newSyslogFile, "syslog", logOptions); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	return nil
}

func (p *PackageOptions) renderLogrotateConfig(ctx context.Context) error {
	logDir := fmt.Sprintf("/var/log/%s", p.Identifier)
	logrotateDirectory := filepath.Join("/etc", "logrotate.d")

	if err := os.MkdirAll(filepath.Join(p.packageRoot, logrotateDirectory), fsutil.DirMode); err != nil {
		return fmt.Errorf("making logrotate.d dir: %w", err)
	}

	logrotatePath := filepath.Join(p.packageRoot, logrotateDirectory, fmt.Sprintf("%s", p.Identifier))
	logrotateFile, err := os.Create(logrotatePath)
	if err != nil {
		return fmt.Errorf("creating logrotate conf file: %w", err)
	}
	defer logrotateFile.Close()

	logOptions := struct {
		LogPath string
		PidPath string
	}{
		LogPath: filepath.Join(logDir, "*.log"),
		PidPath: filepath.Join(p.rootDir, "launcher.pid"),
	}

	logrotateTemplate, err := assets.ReadFile("assets/logrotate.conf")
	if err != nil {
		return fmt.Errorf("failed to get template named %s: %w", "assets/logrotate.conf", err)
	}

	tmpl, err := template.New("logrotate").Parse(string(logrotateTemplate))
	if err != nil {
		return fmt.Errorf("not able to parse logrotate template: %w", err)
	}
	if err := tmpl.ExecuteTemplate(logrotateFile, "logrotate", logOptions); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	return nil
}

// setupInit setups the init scripts.
//
// Note that windows is a special
// case here -- they're not files on disk, instead it's an argument
// passed in to wix. So this is a confusing split.
func (p *PackageOptions) setupInit(ctx context.Context) error {
	if p.target.Init == NoInit {
		return nil
	}

	if p.initOptions == nil {
		return errors.New("Missing initOptions")
	}

	var dir string
	var file string
	var renderFunc func(context.Context, io.Writer, *packagekit.InitOptions) error

	switch {
	case p.target.Platform == Darwin && p.target.Init == LaunchD:
		dir = "/Library/LaunchDaemons"
		file = fmt.Sprintf("com.%s.launcher.plist", p.Identifier)
		renderFunc = packagekit.RenderLaunchd
	case p.target.Platform == Linux && p.target.Init == Systemd:
		// Default to dropping into /lib, it seems more common. But for
		// rpm use /usr/lib.
		dir = "/lib/systemd/system"
		if p.target.Package == Rpm {
			dir = "/usr/lib/systemd/system"
		}
		if p.target.Package == Pacman {
			dir = "/usr/lib/systemd/system"
		}
		file = fmt.Sprintf("launcher.%s.service", p.Identifier)
		renderFunc = packagekit.RenderSystemd
	case p.target.Platform == Linux && p.target.Init == Upstart:
		dir = "/etc/init"
		file = fmt.Sprintf("launcher-%s.conf", p.Identifier)
		renderFunc = func(ctx context.Context, w io.Writer, io *packagekit.InitOptions) error {
			return packagekit.RenderUpstart(ctx, w, io)
		}
	case p.target.Platform == Linux && p.target.Init == UpstartAmazonAMI:
		dir = "/etc/init"
		file = fmt.Sprintf("launcher-%s.conf", p.Identifier)
		renderFunc = func(ctx context.Context, w io.Writer, io *packagekit.InitOptions) error {
			return packagekit.RenderUpstart(ctx, w, io, packagekit.WithUpstartFlavor("amazon-ami"))
		}
	case p.target.Platform == Linux && p.target.Init == Init:
		dir = "/etc/init.d"
		file = fmt.Sprintf("%s-launcher", p.Identifier)
		renderFunc = packagekit.RenderInit
	case p.target.Platform == Windows && p.target.Init == WindowsService:
		// Do nothing, this is handled in the packaging step.
		return nil
	default:
		return fmt.Errorf("Unsupported launcher target %s", p.target.String())
	}

	p.initFile = filepath.Join(dir, file)

	if err := os.MkdirAll(filepath.Join(p.packageRoot, dir), fsutil.DirMode); err != nil {
		return fmt.Errorf("mkdir failed, target %s: %w", p.target.String(), err)
	}

	fh, err := os.Create(filepath.Join(p.packageRoot, p.initFile))
	if err != nil {
		return fmt.Errorf("create filehandle, target %s: %w", p.target.String(), err)
	}
	defer fh.Close()

	if err := renderFunc(ctx, fh, p.initOptions); err != nil {
		return fmt.Errorf("rendering init file (%s), target %s: %w", p.initFile, p.target.String(), err)
	}

	return nil
}

func (p *PackageOptions) setupPrerm(ctx context.Context) error {
	if p.target.Init == NoInit {
		return nil
	}

	var prermTemplate string
	identifier := p.Identifier

	switch {
	case p.target.Platform == Linux && p.target.Init == Systemd:
		prermTemplate = prermSystemdTemplate()
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

	t, err := template.New("prerm").Parse(prermTemplate)
	if err != nil {
		return fmt.Errorf("not able to parse template: %w", err)
	}

	fh, err := os.Create(filepath.Join(p.scriptRoot, "prerm"))
	if err != nil {
		return fmt.Errorf("create prerm filehandle: %w", err)
	}
	defer fh.Close()

	if err := os.Chmod(filepath.Join(p.scriptRoot, "prerm"), 0755); err != nil {
		return fmt.Errorf("chmod prerm: %w", err)
	}

	if err := t.ExecuteTemplate(fh, "prerm", data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}

func (p *PackageOptions) setupPostinst(ctx context.Context) error {
	if p.target.Init == NoInit {
		return nil
	}

	var postinstTemplateName string

	switch {
	case p.target.Platform == Darwin && p.target.Init == LaunchD:
		postinstTemplateName = "postinstall-launchd.sh"
	case p.target.Platform == Linux && p.target.Init == Systemd:
		postinstTemplateName = "postinstall-systemd.sh"
	case p.target.Platform == Linux && (p.target.Init == Upstart || p.target.Init == UpstartAmazonAMI):
		postinstTemplateName = "postinstall-upstart.sh"
	case p.target.Platform == Linux && p.target.Init == Init:
		postinstTemplateName = "postinstall-init.sh"
	default:
		// If we don't match in the case statement, log that we're ignoring
		// the setup, and move on. Don't throw an error.
		// logging
		return nil
	}

	postinstTemplate, err := assets.ReadFile(path.Join("assets", postinstTemplateName))
	if err != nil {
		return fmt.Errorf("Failed to get template named %s: %w", postinstTemplateName, err)
	}

	// installer info will be dumped into the filesystem
	// somewhere. Note that some of these are OS variables (or exec
	// calls) to be expanded at runtime
	installerInfo := map[string]string{
		"identifier":    p.Identifier,
		"installer_id":  "$INSTALL_PKG_SESSION_ID",
		"download_path": "$PACKAGE_PATH",
		"download_file": "$PACKAGE_FILENAME",
		"timestamp":     "$(date +%Y-%m-%dT%T%z)",
		"version":       p.PackageVersion,
		"user":          "$USER",
	}

	jsonBlob, err := json.MarshalIndent(installerInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling installer info: %w", err)
	}

	var data = struct {
		Identifier   string
		Path         string
		InfoFilename string
		InfoJson     string
	}{
		Identifier:   p.Identifier,
		Path:         p.initFile,
		InfoFilename: p.canonicalizePath(filepath.Join(p.confDir, "installer-info.json")),
		InfoJson:     string(jsonBlob),
	}

	funcsMap := template.FuncMap{
		"StringsTrimSuffix": strings.TrimSuffix,
	}

	t, err := template.New("postinstall").Funcs(funcsMap).Parse(string(postinstTemplate))
	if err != nil {
		return fmt.Errorf("not able to parse template: %w", err)
	}

	fh, err := os.Create(filepath.Join(p.scriptRoot, "postinstall"))
	if err != nil {
		return fmt.Errorf("create postinstall filehandle: %w", err)
	}
	defer fh.Close()

	if err := os.Chmod(filepath.Join(p.scriptRoot, "postinstall"), 0755); err != nil {
		return fmt.Errorf("chmod postinst: %w", err)
	}

	if err := t.ExecuteTemplate(fh, "postinstall", data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}

// prermSystemdTemplate returns a template suitable for stopping and
// uninstalling launcher. It's trying to be compatible with both dpkg
// and rpm, so there are slightly more convoluted args.
//
// rpm upgrade: prerm 1
// rpm uninstall: prerm 0
func prermSystemdTemplate() string {
	return `#!/bin/sh
set -e
if [ "$1" = remove -o "$1" = "0" ] ; then
  systemctl stop launcher.{{.Identifier}} || true
  systemctl disable launcher.{{.Identifier}} || true
fi`
}

func (p *PackageOptions) setupDirectories() error {
	switch p.target.Platform {
	case Linux, Darwin:
		p.binDir = filepath.Join("/usr/local", p.Identifier, "bin")
		p.confDir = filepath.Join("/etc", p.Identifier)
		p.rootDir = filepath.Join("/var", p.Identifier, sanitizeHostname(p.Hostname))
	case Windows:
		// On Windows, these paths end up rooted not at `c:`, but instead
		// where the WiX template says. In our case, that's `c:\Program
		// Files\Kolide` These do need the identifier, since we need WiX
		// to take that into account for the guid generation.
		p.binDir = filepath.Join("Launcher-"+p.Identifier, "bin")
		p.confDir = filepath.Join("Launcher-"+p.Identifier, "conf")
		p.rootDir = filepath.Join("Launcher-"+p.Identifier, "data")
	default:
		return fmt.Errorf("Unknown platform %s", string(p.target.Platform))
	}

	for _, d := range []string{p.binDir, p.confDir, p.rootDir} {
		if err := os.MkdirAll(filepath.Join(p.packageRoot, d), fsutil.DirMode); err != nil {
			return fmt.Errorf("create dir (%s) for %s: %w", d, p.target.String(), err)
		}
	}
	return nil
}

func (p *PackageOptions) execOut(ctx context.Context, argv0 string, args ...string) (string, error) {
	// Since PackageOptions is sometimes instantiated directly, set execCC if it's nil.
	if p.execCC == nil {
		p.execCC = exec.CommandContext
	}

	cmd := p.execCC(ctx, argv0, args...)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run command %s %v, stderr=%s: %w", argv0, args, stderr, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// canonicalizePath takes a path, and makes it into a full, absolute,
// path. It is a hack around how the windows install process works,
// and will likely need to be revisited.
//
// The windows process installs using _relative_ paths, which are
// expanded to full paths inside the wix template. However, the flag
// file needs full paths, and is generated here. Thus,
// canonicalizePath encodes some things that should be left as
// install-time variables controlled by wix and windows.
//
// Likely a longer term approach will involve one of:
//  1. pull all the paths into the golang portion.
//  2. Move flag file generation to wix
//  3. utilize some environmental variable
func (p *PackageOptions) canonicalizePath(path string) string {
	if p.target.Package != Msi {
		return path
	}

	return filepath.Join(`C:\Program Files\Kolide`, path)
}

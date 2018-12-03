package packaging

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/kolide/kit/fs"
	"github.com/pkg/errors"
)

type prepareOutput struct {
	PackageRoot string
	ScriptsRoot string
}

func prepare(flavor Flavor, platform string, po PackageOptions) (prepareOutput, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	var prepOut prepareOutput
	so := ServiceOptions{
		ServiceName:       "launcher",
		ServerHostname:    grpcServerForHostname(po.Hostname),
		Insecure:          po.Insecure,
		InsecureGrpc:      po.InsecureGrpc,
		Autoupdate:        po.Autoupdate,
		UpdateChannel:     po.UpdateChannel,
		Control:           po.Control,
		ControlHostname:   po.ControlHostname,
		DisableControlTLS: po.DisableControlTLS,
		CertPins:          po.CertPins,
		InitialRunner:     po.InitialRunner,
	}
	if po.UpdateChannel == "" {
		so.UpdateChannel = "stable"
	}

	packageRoot, err := ioutil.TempDir("/tmp", fmt.Sprintf("package_%s.packageRoot", flavor))
	if err != nil {
		return prepOut, errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	prepOut.PackageRoot = packageRoot

	so.RootDirectory, err = rootDirectory(packageRoot, po.Identifier, po.Hostname, flavor)
	if err != nil {
		return prepOut, err
	}

	binDir, err := binaryDirectory(packageRoot, po.Identifier, flavor)
	if err != nil {
		return prepOut, err
	}
	bp := binaryPaths(binDir, flavor)
	so.LauncherPath, so.OsquerydPath = bp.Launcher, bp.Osqueryd

	confDir, err := configurationDirectory(packageRoot, po.Identifier, flavor)
	if err != nil {
		return prepOut, err
	}
	so.SecretPath = filepath.Join(confDir, "secret")
	if !po.OmitSecret {
		if err := ioutil.WriteFile(
			filepath.Join(packageRoot, so.SecretPath),
			[]byte(po.Secret),
			secretPerms,
		); err != nil {
			return prepOut, errors.Wrap(err, "could not write secret string to file for packaging")
		}
	}

	if po.RootPEM != "" {
		so.RootPEM = filepath.Join(confDir, "roots.pem")
		if err := fs.CopyFile(po.RootPEM, filepath.Join(packageRoot, so.RootPEM)); err != nil {
			return prepOut, errors.Wrap(err, "copy root PEM")
		}

		if err := os.Chmod(filepath.Join(packageRoot, so.RootPEM), 0600); err != nil {
			return prepOut, errors.Wrap(err, "chmod root PEM")
		}
	}

	sp, err := servicePaths(packageRoot, po.Identifier, flavor)
	if err != nil {
		return prepOut, err
	}

	serviceFile, err := os.Create(filepath.Join(packageRoot, sp.servicePath))
	if err != nil {
		return prepOut, err
	}
	defer serviceFile.Close()
	if err := so.Render(serviceFile, flavor); err != nil {
		return prepOut, err
	}

	localOsquerydPath, err := FetchOsquerydBinary(po.CacheDir, po.OsqueryVersion, platform)
	if err != nil {
		return prepOut, errors.Wrap(err, "could not fetch path to osqueryd binary")
	}
	if err := fs.CopyFile(
		localOsquerydPath,
		filepath.Join(packageRoot, so.OsquerydPath),
	); err != nil {
		return prepOut, err
	}

	if err := fs.CopyFile( // TODO: also fetch
		filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/build", platform, "launcher"),
		filepath.Join(packageRoot, so.LauncherPath),
	); err != nil {
		return prepOut, err
	}

	if err := fs.CopyFile( // TODO: also fetch
		filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/build", platform, "osquery-extension.ext"),
		filepath.Join(packageRoot, bp.OsqueryExtension),
	); err != nil {
		return prepOut, err
	}

	if flavor == LaunchD {
		// add logging things
		so.LogDirectory = filepath.Join("/var/log", po.Identifier)
		newSysLogDirectory := filepath.Join("/etc", "newsyslog.d")
		if err := os.MkdirAll(filepath.Join(packageRoot, newSysLogDirectory), fs.DirMode); err != nil {
			return prepOut, err
		}
		newSysLogPath := filepath.Join(packageRoot, newSysLogDirectory, fmt.Sprintf("%s.conf", po.Identifier))
		newSyslogFile, err := os.Create(newSysLogPath)
		if err != nil {
			return prepOut, err
		}
		defer newSyslogFile.Close()
		logOptions := newSyslogTemplateOptions{
			LogPath: filepath.Join(so.LogDirectory, "*.log"),
			PidPath: filepath.Join(so.RootDirectory, "launcher.pid"),
		}
		if err := renderNewSyslogConfig(newSyslogFile, &logOptions); err != nil {
			return prepOut, err
		}
	}

	scriptDir, err := ioutil.TempDir("", "scriptDir")
	if err != nil {
		return prepOut, err
	}
	prepOut.ScriptsRoot = scriptDir
	postinstallFile, err := os.Create(filepath.Join(scriptDir, "postinstall"))
	if err != nil {
		return prepOut, err
	}
	if err := postinstallFile.Chmod(0755); err != nil {
		return prepOut, err
	}
	defer postinstallFile.Close()

	// handle postinstalls
	switch flavor {
	case LaunchD:
		postinstallOpts := &postinstallTemplateOptions{
			LaunchDaemonDirectory: sp.dir,
			LaunchDaemonName:      sp.serviceName,
		}
		if err := renderPostinstall(postinstallFile, postinstallOpts); err != nil {
			return prepOut, err
		}
	case SystemD:
		postInstallLauncherContents := `#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable launcher
systemctl restart launcher`
		fmt.Fprintf(postinstallFile, postInstallLauncherContents)

	case Init:
		postInstallLauncherContents := `#!/bin/bash
sudo service launcher restart`
		fmt.Fprintf(postinstallFile, postInstallLauncherContents)
	}

	return prepOut, nil
}

func rootDirectory(packageRoot, identifier, hostname string, flavor Flavor) (string, error) {
	var dir string
	switch flavor {
	case LaunchD, SystemD, Init:
		dir = filepath.Join("/var", identifier, sanitizeHostname(hostname))
	}
	err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode)
	return dir, errors.Wrapf(err, "create root dir for %s", flavor)
}

func binaryDirectory(packageRoot, identifier string, flavor Flavor) (string, error) {
	var dir string
	switch flavor {
	case LaunchD, SystemD, Init:
		dir = filepath.Join("/usr/local", identifier, "bin")
	}
	err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode)
	return dir, errors.Wrapf(err, "create binary dir for %s", flavor)
}

func configurationDirectory(packageRoot, identifier string, flavor Flavor) (string, error) {
	var dir string
	switch flavor {
	case LaunchD, SystemD, Init:
		dir = filepath.Join("/etc", identifier)
	}
	err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode)
	return dir, errors.Wrapf(err, "create config dir for %s", flavor)
}

type binPath struct {
	Launcher         string
	Osqueryd         string
	OsqueryExtension string
}

func binaryPaths(binaryDirectory string, flavor Flavor) binPath {
	var bp binPath
	switch flavor {
	case LaunchD, SystemD, Init:
		bp.Launcher = filepath.Join(binaryDirectory, "launcher")
		bp.Osqueryd = filepath.Join(binaryDirectory, "osqueryd")
		bp.OsqueryExtension = filepath.Join(binaryDirectory, "osquery-extension.ext")
	}
	return bp
}

type svcPath struct {
	dir         string
	serviceName string
	servicePath string
}

func servicePaths(packageRoot, identifier string, flavor Flavor) (svcPath, error) {
	var sp svcPath
	switch flavor {
	case LaunchD:
		sp.dir = "/Library/LaunchDaemons"
		sp.serviceName = fmt.Sprintf("com.%s.launcher", identifier)
		sp.servicePath = filepath.Join(sp.dir, fmt.Sprintf("%s.plist", sp.serviceName))
	case SystemD:
		sp.dir = "/etc/systemd/system"
		sp.servicePath = "launcher.service"
	case Init:
		sp.dir = "/etc/init.d/"
		sp.servicePath = "launcher"
	}
	err := os.MkdirAll(filepath.Join(packageRoot, sp.dir), fs.DirMode)
	return sp, err
}

type postinstallTemplateOptions struct {
	LaunchDaemonDirectory string
	LaunchDaemonName      string
}

func renderPostinstall(w io.Writer, options *postinstallTemplateOptions) error {
	postinstallTemplate := `#!/bin/bash

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop {{.LaunchDaemonName}}

sleep 5

/bin/launchctl unload {{.LaunchDaemonDirectory}}/{{.LaunchDaemonName}}.plist
/bin/launchctl load {{.LaunchDaemonDirectory}}/{{.LaunchDaemonName}}.plist`
	t, err := template.New("postinstall").Parse(postinstallTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse postinstall template")
	}
	return t.ExecuteTemplate(w, "postinstall", options)
}

type newSyslogTemplateOptions struct {
	LogPath string
	PidPath string
}

func renderNewSyslogConfig(w io.Writer, options *newSyslogTemplateOptions) error {
	syslogTemplate := `# logfilename          [owner:group]    mode count size when  flags [/pid_file] [sig_num]
{{.LogPath}}               640  3  4000   *   G  {{.PidPath}} 15`
	t, err := template.New("syslog").Parse(syslogTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse postinstall template")
	}
	return t.ExecuteTemplate(w, "syslog", options)
}

// grpcServerForHostname returns the gRPC server hostname given a web address
// that was serving the website itself
func grpcServerForHostname(hostname string) string {
	switch hostname {
	case "localhost:5000":
		return "localhost:8800"
	case "master.cloud.kolide.net":
		return "master-grpc.cloud.kolide.net:443"
	case "kolide.co", "kolide.com":
		return "launcher.kolide.co:443"
	default:
		if strings.Contains(hostname, ":") {
			return hostname
		} else {
			return fmt.Sprintf("%s:443", hostname)
		}
	}
}

// sanitizeHostname will replace any ":" characters in a given hostname with "-"
// This is useful because ":" is not a valid character for file paths.
func sanitizeHostname(hostname string) string {
	return strings.Replace(hostname, ":", "-", -1)
}

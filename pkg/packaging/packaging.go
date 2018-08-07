package packaging

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/kolide/kit/fs"
	"github.com/pkg/errors"
)

const (
	// Enroll secret should be readable only by root
	secretPerms = 0600
)

// PackagePaths is a simple wrapper for passing around the paths of packages for
// various platforms
type PackagePaths struct {
	MacOS string
	Deb   string
	RPM   string
}

// CreatePackages will create a launcher macOS package. The output paths of the
// packages are returned and an error if the operation was not successful.
func CreatePackages(
	packageVersion,
	osqueryVersion,
	hostname,
	secret,
	macPackageSigningKey string,
	insecure,
	insecureGrpc,
	autoupdate bool,
	updateChannel string,
	control bool,
	controlHostname string,
	disableControlTLS bool,
	identifier string,
	omitSecret bool,
	systemd bool,
	certPins,
	rootPEM string,
) (*PackagePaths, error) {
	macPkgDestinationPath, err := CreateMacPackage(
		packageVersion,
		osqueryVersion,
		hostname,
		secret,
		macPackageSigningKey,
		insecure,
		insecureGrpc,
		autoupdate,
		updateChannel,
		control,
		controlHostname,
		disableControlTLS,
		identifier,
		omitSecret,
		certPins,
		rootPEM,
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate macOS package")
	}

	debDestinationPath, rpmDestinationPath, err := CreateLinuxPackages(
		packageVersion,
		osqueryVersion,
		hostname,
		secret,
		insecure,
		insecureGrpc,
		autoupdate,
		updateChannel,
		control,
		controlHostname,
		disableControlTLS,
		identifier,
		omitSecret,
		systemd,
		certPins,
		rootPEM,
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate linux packages")
	}

	return &PackagePaths{
		MacOS: macPkgDestinationPath,
		Deb:   debDestinationPath,
		RPM:   rpmDestinationPath,
	}, nil
}

func createInitFiles(opts *initTemplateOptions, serviceDirectory string, initFileName string, packageRoot string, binaryDirectory string, postInstallScript string, postInstallLauncherContents string, systemd bool) error {
	// Create the init file for the launcher service
	initPath := filepath.Join(serviceDirectory, initFileName)
	initFile, err := os.Create(filepath.Join(packageRoot, initPath))
	if err != nil {
		return errors.Wrap(err, "could not create init system file")
	}
	defer initFile.Close()
	if err := initFile.Chmod(0755); err != nil {
		return errors.Wrap(err, "could not make postinstall script executable")
	}
	if systemd {
		if err := renderSystemdService(initFile, opts); err != nil {
			return errors.Wrap(err, "could not render init system file")
		}
	} else {
		if err := renderInitService(initFile, opts); err != nil {
			return errors.Wrap(err, "could not render init system file")
		}
	}

	postInstallLauncherFile, err := os.Create(
		filepath.Join(packageRoot, binaryDirectory, postInstallScript),
	)
	if err != nil {
		return errors.Wrap(err, "could not create the post install script")
	}
	fmt.Fprintf(postInstallLauncherFile, postInstallLauncherContents)
	postInstallLauncherFile.Close()
	return nil
}

func CreateLinuxPackages(
	packageVersion,
	osqueryVersion,
	hostname,
	secret string,
	insecure,
	insecureGrpc,
	autoupdate bool,
	updateChannel string,
	control bool,
	controlHostname string,
	disableControlTLS bool,
	identifier string,
	omitSecret bool,
	systemd bool,
	certPins,
	rootPEM string,
) (string, string, error) {
	postInstallScript := "launcher-installer"
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("/tmp", "CreateLinuxPackages.packageRoot_")
	if err != nil {
		return "", "", errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(packageRoot)

	// Here, we must create the directory structure of our package.
	// First, we create all of the directories that we will need:
	rootDirectory := filepath.Join("/var", identifier, sanitizeHostname(hostname))
	binaryDirectory := filepath.Join("/usr/local", identifier, "bin")
	configurationDirectory := filepath.Join("/etc", identifier)
	serviceDirectory := "/etc/systemd/system"

	if !systemd {
		serviceDirectory = "/etc/init.d/"
	}

	pathsToCreate := []string{
		rootDirectory,
		binaryDirectory,
		configurationDirectory,
		serviceDirectory,
	}

	for _, pathToCreate := range pathsToCreate {
		err = os.MkdirAll(filepath.Join(packageRoot, pathToCreate), fs.DirMode)
		if err != nil {
			return "", "", errors.Wrap(err, fmt.Sprintf("could not make directory %s/%s", packageRoot, pathToCreate))
		}
	}

	// Next we create each file that gets laid down as a result of the package
	// installation:

	// The initial osqueryd binary
	osquerydPath, err := FetchOsquerydBinary(osqueryVersion, "linux")
	if err != nil {
		return "", "", errors.Wrap(err, "could not fetch path to osqueryd binary")
	}

	err = fs.CopyFile(osquerydPath, filepath.Join(packageRoot, binaryDirectory, "osqueryd"))
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the osqueryd binary to the packaging root")
	}

	// The secret which the user will use to authenticate to the cloud
	secretPath := filepath.Join(configurationDirectory, "secret")

	if !omitSecret {
		err = ioutil.WriteFile(filepath.Join(packageRoot, secretPath), []byte(secret), secretPerms)
		if err != nil {
			return "", "", errors.Wrap(err, "could not write secret string to file for packaging")
		}
	}

	var rootPEMPath string
	if rootPEM != "" {
		rootPEMPath = filepath.Join(configurationDirectory, "roots.pem")

		if err := fs.CopyFile(rootPEM, filepath.Join(packageRoot, rootPEMPath)); err != nil {
			return "", "", errors.Wrap(err, "copy root PEM")
		}
		if err := os.Chmod(filepath.Join(packageRoot, rootPEMPath), 0600); err != nil {
			return "", "", errors.Wrap(err, "chmod root PEM")
		}
	}

	if updateChannel == "" {
		updateChannel = "stable"
	}

	opts := &initTemplateOptions{
		LaunchDaemonName:  "launcher",
		ServerHostname:    grpcServerForHostname(hostname),
		RootDirectory:     rootDirectory,
		SecretPath:        secretPath,
		OsquerydPath:      filepath.Join(binaryDirectory, "osqueryd"),
		LauncherPath:      filepath.Join(binaryDirectory, "launcher"),
		Insecure:          insecure,
		InsecureGrpc:      insecureGrpc,
		Autoupdate:        autoupdate,
		UpdateChannel:     updateChannel,
		Control:           control,
		ControlHostname:   controlHostname,
		DisableControlTLS: disableControlTLS,
		CertPins:          certPins,
		RootPEM:           rootPEMPath,
	}

	if systemd {
		initFileName := "launcher.service"
		postInstallLauncherContents := `#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable launcher
systemctl restart launcher`
		createInitFiles(opts, serviceDirectory, initFileName, packageRoot, binaryDirectory, postInstallScript, postInstallLauncherContents, systemd)

	} else { //not systemd, so assume init
		initFileName := "launcher"
		// The post install step
		postInstallLauncherContents := `#!/bin/bash
sudo service launcher restart`
		createInitFiles(opts, serviceDirectory, initFileName, packageRoot, binaryDirectory, postInstallScript, postInstallLauncherContents, systemd)
	}

	// The initial launcher (and extension) binary
	err = fs.CopyFile(
		filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/build/linux/launcher"),
		filepath.Join(packageRoot, binaryDirectory, "launcher"),
	)
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the launcher binary to the packaging root")
	}

	err = fs.CopyFile(
		filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/build/linux/osquery-extension.ext"),
		filepath.Join(packageRoot, binaryDirectory, "osquery-extension.ext"),
	)
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the osquery-extension binary to the packaging root")
	}

	outputPathDir, err := ioutil.TempDir("/tmp", "packages_")
	if err != nil {
		return "", "", errors.Wrap(err, "could not create final output directory for package")
	}

	debOutputFilename := fmt.Sprintf("launcher-linux-%s.deb", packageVersion)
	debOutputPath := filepath.Join(outputPathDir, debOutputFilename)

	rpmOutputFilename := fmt.Sprintf("launcher-linux-%s.rpm", packageVersion)
	rpmOutputPath := filepath.Join(outputPathDir, rpmOutputFilename)

	// Create the packages
	containerPackageRoot := "/pkgroot"

	afterInstall := filepath.Join(containerPackageRoot, binaryDirectory, postInstallScript)

	debCmd := exec.Command(
		"docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:%s", packageRoot, containerPackageRoot),
		"-v", fmt.Sprintf("%s:/out", outputPathDir),
		"kolide/fpm",
		"fpm",
		"-s", "dir",
		"-t", "deb",
		"-n", "launcher",
		"-v", packageVersion,
		"-p", filepath.Join("/out", debOutputFilename),
		"--after-install", afterInstall,
		"-C", containerPackageRoot,
		".",
	)
	stderr := new(bytes.Buffer)
	debCmd.Stderr = stderr
	if err := debCmd.Run(); err != nil {
		io.Copy(os.Stderr, stderr)
		return "", "", errors.Wrap(err, "could not create deb package")
	}

	rpmCmd := exec.Command(
		"docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:%s", packageRoot, containerPackageRoot),
		"-v", fmt.Sprintf("%s:/out", outputPathDir),
		"kolide/fpm",
		"fpm",
		"-s", "dir",
		"-t", "rpm",
		"-n", "launcher",
		"-v", packageVersion,
		"-p", filepath.Join("/out", rpmOutputFilename),
		"--after-install", afterInstall,
		"-C", containerPackageRoot,
		".",
	)
	stderr = new(bytes.Buffer)
	rpmCmd.Stderr = stderr
	if err := rpmCmd.Run(); err != nil {
		io.Copy(os.Stderr, stderr)
		return "", "", errors.Wrap(err, "could not create rpm package")
	}

	return debOutputPath, rpmOutputPath, nil
}

func CreateMacPackage(
	packageVersion,
	osqueryVersion,
	hostname,
	secret,
	macPackageSigningKey string,
	insecure,
	insecureGrpc,
	autoupdate bool,
	updateChannel string,
	control bool,
	controlHostname string,
	disableControlTLS bool,
	identifier string,
	omitSecret bool,
	certPins,
	rootPEM string,
) (string, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("/tmp", "CreateMacPackage.packageRoot")
	if err != nil {
		return "", errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(packageRoot)

	// a macOS package is basically the ability to lay an additive addition of
	// files to the file system, as well as specify exeuctable scripts at certain
	// points in the installation process (before install, after, etc.).

	// Here, we must create the directory structure of our package.
	// First, we create all of the directories that we will need:
	rootDirectory := filepath.Join("/var", identifier, sanitizeHostname(hostname))
	binaryDirectory := filepath.Join("/usr/local", identifier, "bin")
	launcherPath := filepath.Join(binaryDirectory, "launcher")
	osquerydPath := filepath.Join(binaryDirectory, "osqueryd")
	configurationDirectory := filepath.Join("/etc", identifier)
	secretPath := filepath.Join(configurationDirectory, "secret")
	logDirectory := filepath.Join("/var/log", identifier)
	launchDaemonDirectory := "/Library/LaunchDaemons"
	launchDaemonName := fmt.Sprintf("com.%s.launcher", identifier)
	pathsToCreate := []string{
		rootDirectory,
		binaryDirectory,
		configurationDirectory,
		logDirectory,
		launchDaemonDirectory,
	}
	for _, pathToCreate := range pathsToCreate {
		err = os.MkdirAll(filepath.Join(packageRoot, pathToCreate), fs.DirMode)
		if err != nil {
			return "", errors.Wrapf(err, "could not make directory %s/%s", packageRoot, pathToCreate)
		}
	}

	// Next we create each file that gets laid down as a result of the package
	// installation:

	// The initial osqueryd binary
	localOsquerydPath, err := FetchOsquerydBinary(osqueryVersion, "darwin")
	if err != nil {
		return "", errors.Wrap(err, "could not fetch path to osqueryd binary")
	}

	err = fs.CopyFile(localOsquerydPath, filepath.Join(packageRoot, osquerydPath))
	if err != nil {
		return "", errors.Wrap(err, "could not copy the osqueryd binary to the packaging root")
	}

	// The initial launcher (and extension) binary
	err = fs.CopyFile(
		filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/build/darwin/launcher"),
		filepath.Join(packageRoot, launcherPath),
	)
	if err != nil {
		return "", errors.Wrap(err, "could not copy the launcher binary to the packaging root")
	}

	err = fs.CopyFile(
		filepath.Join(fs.Gopath(), "src/github.com/kolide/launcher/build/darwin/osquery-extension.ext"),
		filepath.Join(packageRoot, binaryDirectory, "osquery-extension.ext"),
	)
	if err != nil {
		return "", errors.Wrap(err, "could not copy the osquery-extension binary to the packaging root")
	}

	var rootPEMPath string
	if rootPEM != "" {
		rootPEMPath = filepath.Join(configurationDirectory, "roots.pem")

		if err := fs.CopyFile(rootPEM, filepath.Join(packageRoot, rootPEMPath)); err != nil {
			return "", errors.Wrap(err, "copy root PEM")
		}

		if err := os.Chmod(filepath.Join(packageRoot, rootPEMPath), 0600); err != nil {
			return "", errors.Wrap(err, "chmod root PEM")
		}
	}

	// The LaunchDaemon which will connect the launcher to the cloud
	launchDaemonFile, err := os.Create(filepath.Join(packageRoot, launchDaemonDirectory, fmt.Sprintf("%s.plist", launchDaemonName)))
	if err != nil {
		return "", errors.Wrap(err, "could not open the LaunchDaemon path for writing")
	}

	if updateChannel == "" {
		updateChannel = "stable"
	}

	opts := &launchDaemonTemplateOptions{
		ServerHostname:    grpcServerForHostname(hostname),
		RootDirectory:     rootDirectory,
		LauncherPath:      launcherPath,
		OsquerydPath:      osquerydPath,
		LogDirectory:      logDirectory,
		SecretPath:        secretPath,
		LaunchDaemonName:  launchDaemonName,
		Insecure:          insecure,
		InsecureGrpc:      insecureGrpc,
		Autoupdate:        autoupdate,
		UpdateChannel:     updateChannel,
		Control:           control,
		ControlHostname:   controlHostname,
		DisableControlTLS: disableControlTLS,
		CertPins:          certPins,
		RootPEM:           rootPEMPath,
	}
	if err := renderLaunchDaemon(launchDaemonFile, opts); err != nil {
		return "", errors.Wrap(err, "could not write LaunchDaemon content to file")
	}

	// The secret which the user will use to authenticate to the server
	if !omitSecret {
		err = ioutil.WriteFile(filepath.Join(packageRoot, secretPath), []byte(secret), secretPerms)
		if err != nil {
			return "", errors.Wrap(err, "could not write secret string to file for packaging")
		}
	}

	// Finally, now that the final directory structure of the package is
	// represented, we can create the package

	// First, we render the macOS post-install script
	scriptDir, err := ioutil.TempDir("", "scriptDir")
	if err != nil {
		return "", errors.Wrap(err, "could not create temp directory for the macOS packaging script directory")
	}
	defer os.RemoveAll(scriptDir)

	postinstallFile, err := os.Create(filepath.Join(scriptDir, "postinstall"))
	if err != nil {
		return "", errors.Wrap(err, "could not open the postinstall file for writing")
	}
	if err := postinstallFile.Chmod(0755); err != nil {
		return "", errors.Wrap(err, "could not make postinstall script executable")
	}
	postinstallOpts := &postinstallTemplateOptions{
		LaunchDaemonDirectory: launchDaemonDirectory,
		LaunchDaemonName:      launchDaemonName,
	}
	if err := renderPostinstall(postinstallFile, postinstallOpts); err != nil {
		return "", errors.Wrap(err, "could not render postinstall script context to file")
	}

	outputPathDir, err := ioutil.TempDir("/tmp", "packaging_")
	outputFile := fmt.Sprintf("launcher-darwin-%s.pkg", packageVersion)
	outputPath := filepath.Join(outputPathDir, outputFile)
	if err != nil {
		return "", errors.Wrap(err, "could not create final output directory for package")
	}

	distributionFileName := "distribution.xml"
	distributionFilePath := filepath.Join(outputPathDir, distributionFileName)
	distributionFile, err := os.Create(distributionFilePath)
	distrOutputFile := fmt.Sprintf("launcher-darwin-%s-distribution.pkg", packageVersion)
	distrOutputPath := filepath.Join(outputPathDir, distrOutputFile)
	distributionOpts := &distributionTemplateOptions{
		PackageVersion:    packageVersion,
		PackageFileName:   outputFile,
		Identifier:        identifier,
	}
	if err := renderDistributionFile(distributionFile, distributionOpts); err != nil {
		return "", errors.Wrap(err, "could not render distribution file to disk")
	}

	// Build the macOS package
	err = pkgbuild(
		packageRoot,
		scriptDir,
		launchDaemonName,
		packageVersion,
		macPackageSigningKey,
		outputPath,
	)
	if err != nil {
		return "", errors.Wrap(err, "could not create macOS package")
	}

	// Add the distribution file
	err = productbuild(
		outputPathDir,
		distributionFileName,
		outputFile,
		macPackageSigningKey,
		distrOutputPath,
	)
	if err != nil {
		return "", errors.Wrap(err, "could not produce a macOS distribution package")
	}

	return distrOutputPath, nil
}

// systemdTemplateOptions is a struct which contains dynamic systemd
// parameters that will be rendered into a template in renderInitdService
type initTemplateOptions struct {
	ServerHostname    string
	RootDirectory     string
	LauncherPath      string
	OsquerydPath      string
	LogDirectory      string
	SecretPath        string
	LaunchDaemonName  string
	InsecureGrpc      bool
	Insecure          bool
	Autoupdate        bool
	UpdateChannel     string
	Control           bool
	ControlHostname   string
	DisableControlTLS bool
	CertPins          string
	RootPEM           string
}

//renderInitService renders an init service to start and schedule the launcher
func renderInitService(w io.Writer, options *initTemplateOptions) error {
	initdTemplate := `#!/bin/sh
set -e
NAME="{{.LaunchDaemonName}}"
DAEMON="{{.LauncherPath}}"
DAEMON_OPTS="--root_directory={{.RootDirectory}} \
--hostname={{.ServerHostname}} \
--enroll_secret_path={{.SecretPath}} \{{if .InsecureGrpc}}
--insecure_grpc \{{end}}{{if .Insecure}}
--insecure \{{end}}{{if .Autoupdate}}
--autoupdate \
--update_channel={{.UpdateChannel}} \{{end}}{{if .CertPins}}
--cert_pins={{.CertPins}} \{{end}}{{if .RootPEM}}
--root_pem={{.RootPEM}} \{{end}}
--osqueryd_path={{.OsquerydPath}}"

export PATH="${PATH:+$PATH:}/usr/sbin:/sbin"

is_running() {
    start-stop-daemon --status --exec $DAEMON
}
case "$1" in
  start)
        echo "Starting daemon: "$NAME
        start-stop-daemon --start --quiet --background --exec $DAEMON -- $DAEMON_OPTS
        ;;
  stop)
        echo "Stopping daemon: "$NAME
        start-stop-daemon --stop --quiet --oknodo --exec $DAEMON
        ;;
  restart)
        echo "Restarting daemon: "$NAME
        start-stop-daemon --stop --quiet --oknodo --retry 30 --exec $DAEMON
        start-stop-daemon --start --quiet --background --exec $DAEMON -- $DAEMON_OPTS
        ;;
  status)
    if is_running; then
        echo "Running"
    else
        echo "Stopped"
        exit 1
    fi
    ;;
  *)
        echo "Usage: "$1" {start|stop|restart|status}"
        exit 1
esac

exit 0
`

	t, err := template.New("initd").Parse(initdTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse initd template")
	}
	return t.ExecuteTemplate(w, "initd", options)
}

// renderSystemdService renders a systemd service to start and schedule the launcher.
func renderSystemdService(w io.Writer, options *initTemplateOptions) error {
	systemdTemplate :=
		`[Unit]
Description=The Kolide Launcher
After=network.service syslog.service

[Service]
ExecStart={{.LauncherPath}} \
--root_directory={{.RootDirectory}} \
--hostname={{.ServerHostname}} \
--enroll_secret_path={{.SecretPath}} \{{if .InsecureGrpc}}
--insecure_grpc \{{end}}{{if .Insecure}}
--insecure \{{end}}{{if .Control}}
--control \
--control_hostname={{.ControlHostname}} \{{end}}{{if .DisableControlTLS}}
--disable_control_tls \{{end}}{{if .Autoupdate}}
--autoupdate \
--update_channel={{.UpdateChannel}} \{{end}}{{if .CertPins }}
--cert_pins={{.CertPins}} \{{end}}{{if .RootPEM}}
--root_pem={{.RootPEM}} \{{end}}
--osqueryd_path={{.OsquerydPath}}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target`
	t, err := template.New("systemd").Parse(systemdTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse systemd template")
	}
	return t.ExecuteTemplate(w, "systemd", options)
}

// launchDaemonTemplateOptions is a struct which contains dynamic LaunchDaemon
// parameters that will be rendered into a template in renderLaunchDaemon
type launchDaemonTemplateOptions struct {
	ServerHostname    string
	RootDirectory     string
	LauncherPath      string
	OsquerydPath      string
	LogDirectory      string
	SecretPath        string
	LaunchDaemonName  string
	InsecureGrpc      bool
	Insecure          bool
	Autoupdate        bool
	UpdateChannel     string
	Control           bool
	ControlHostname   string
	DisableControlTLS bool
	CertPins          string
	RootPEM           string
}

// renderLaunchDaemon renders a LaunchDaemon to start and schedule the launcher.
func renderLaunchDaemon(w io.Writer, options *launchDaemonTemplateOptions) error {
	launchDaemonTemplate :=
		`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>{{.LaunchDaemonName}}</string>
        <key>EnvironmentVariables</key>
        <dict>
            <key>KOLIDE_LAUNCHER_ROOT_DIRECTORY</key>
            <string>{{.RootDirectory}}</string>
            <key>KOLIDE_LAUNCHER_HOSTNAME</key>
            <string>{{.ServerHostname}}</string>
            <key>KOLIDE_LAUNCHER_ENROLL_SECRET_PATH</key>
            <string>{{.SecretPath}}</string>
            <key>KOLIDE_LAUNCHER_OSQUERYD_PATH</key>
            <string>{{.OsquerydPath}}</string>{{if .Control}}
            <key>KOLIDE_CONTROL_HOSTNAME</key>
            <string>{{.ControlHostname}}</string>{{end}}{{if .Autoupdate}}
            <key>KOLIDE_LAUNCHER_UPDATE_CHANNEL</key>
            <string>{{.UpdateChannel}}</string>{{end}}{{if .CertPins }}
            <key>KOLIDE_LAUNCHER_CERT_PINS</key>
            <string>{{.CertPins}}</string>{{end}}{{if .RootPEM }}
            <key>KOLIDE_LAUNCHER_ROOT_PEM</key>
            <string>{{.RootPEM}}</string>{{end}}
        </dict>
        <key>KeepAlive</key>
        <dict>
            <key>PathState</key>
            <dict>
                <key>{{.SecretPath}}</key>
                <true/>
            </dict>
        </dict>
        <key>ThrottleInterval</key>
        <integer>60</integer>
        <key>ProgramArguments</key>
        <array>
            <string>{{.LauncherPath}}</string>
            {{if .InsecureGrpc}}
            <string>--insecure_grpc</string>
			{{end}}
			{{if .Insecure}}
            <string>--insecure</string>{{end}}
			{{if .Autoupdate}}
            <string>--autoupdate</string>
			{{end}}
			{{if .Control}}
            <string>--control</string>
			{{end}}
			{{if .DisableControlTLS}}
            <string>--disable_control_tls</string>
			{{end}}
        </array>
        <key>StandardErrorPath</key>
        <string>{{.LogDirectory}}/launcher-stderr.log</string>
        <key>StandardOutPath</key>
        <string>{{.LogDirectory}}/launcher-stdout.log</string>
    </dict>
</plist>`
	t, err := template.New("LaunchDaemon").Parse(launchDaemonTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse LaunchDaemon template")
	}
	return t.ExecuteTemplate(w, "LaunchDaemon", options)
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

// productbuild runs the following productbuild command:
//   productbuild \
//     --distribution ${distributionFile} \
//     --package-path ${packageFile} \
//     [--sign ${macPackageSigningKey}] \
//     ${outputPath}
func productbuild(packageDir, distributionFile, packageFile, macPackageSigningKey, outputPath string) error {
	args := []string{
		"--distribution", distributionFile,
		"--package-path", packageFile,
	}

	if macPackageSigningKey != "" {
		args = append(args, "--sign", macPackageSigningKey)
	}

	args = append(args, outputPath)
	cmd := exec.Command("productbuild", args...)
	cmd.Dir = packageDir
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		io.Copy(os.Stderr, stderr)
		return err
	}
	return nil
}

// pkgbuild runs the following pkgbuild command:
//   pkgbuild \
//     --root ${packageRoot} \
//     --scripts ${scriptsRoot} \
//     --identifier ${identifier} \
//     --version ${packageVersion} \
//     ${outputPath}
func pkgbuild(packageRoot, scriptsRoot, identifier, version, macPackageSigningKey, outputPath string) error {
	args := []string{
		"--root", packageRoot,
		"--scripts", scriptsRoot,
		"--identifier", identifier,
		"--version", version,
	}

	if macPackageSigningKey != "" {
		args = append(args, "--sign", macPackageSigningKey)
	}

	args = append(args, outputPath)
	cmd := exec.Command("pkgbuild", args...)
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		io.Copy(os.Stderr, stderr)
		return err
	}
	return nil
}

// distributionTemplateOptions is a struct which contains dynamic distribution
// file parameters that will be rendered into a template.
type distributionTemplateOptions struct {
	PackageVersion    string
	PackageFileName   string
	Identifier        string
}

// renderDistributionFile renders a distribution file to add to launcher's template.
func renderDistributionFile(w io.Writer, options *distributionTemplateOptions) error {
	distributionTemplate :=
		`<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="1">
    <pkg-ref id="com.{{.Identifier}}.launcher"/>
    <options customize="never" require-scripts="false"/>
    <choices-outline>
        <line choice="default">
            <line choice="com.{{.Identifier}}.launcher"/>
        </line>
    </choices-outline>
    <choice id="default"/>
    <choice id="com.{{.Identifier}}.launcher" visible="false">
        <pkg-ref id="com.{{.Identifier}}.launcher"/>
    </choice>
    <pkg-ref id="com.{{.Identifier}}.launcher" version="{{.PackageVersion}}" onConclusion="none">{{.PackageFileName}}</pkg-ref>
</installer-gui-script>`
	t, err := template.New("Distribution").Parse(distributionTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse Distribution template")
	}
	return t.ExecuteTemplate(w, "Distribution", options)
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

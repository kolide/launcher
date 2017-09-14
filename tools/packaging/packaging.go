package packaging

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/kolide/kit/version"
	"github.com/pkg/errors"
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
func CreatePackages(osqueryVersion, hostname, secret, macPackageSigningKey string, insecure, insecureGrpc bool, identifier string) (*PackagePaths, error) {
	macPkgDestinationPath, err := createMacPackage(osqueryVersion, hostname, secret, macPackageSigningKey, insecure, insecureGrpc, identifier)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate macOS package")
	}

	debDestinationPath, rpmDestinationPath, err := createLinuxPackages(osqueryVersion, hostname, secret, insecure, insecureGrpc, identifier)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate linux packages")
	}

	return &PackagePaths{
		MacOS: macPkgDestinationPath,
		Deb:   debDestinationPath,
		RPM:   rpmDestinationPath,
	}, nil
}

func createLinuxPackages(osqueryVersion, hostname, secret string, insecure, insecureGrpc bool, identifier string) (string, string, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("/tmp", "createLinuxPackages.packageRoot_")
	if err != nil {
		return "", "", errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(packageRoot)

	// Here, we must create the directory structure of our package.
	// First, we create all of the directories that we will need:
	rootDirectory := filepath.Join("/var", identifier, sanitizeHostname(hostname))
	binaryDirectory := filepath.Join("/usr/local", identifier, "bin")
	configurationDirectory := filepath.Join("/etc", identifier)
	logDirectory := filepath.Join("/var/log", identifier)
	pathsToCreate := []string{
		rootDirectory,
		binaryDirectory,
		configurationDirectory,
		logDirectory,
	}

	for _, pathToCreate := range pathsToCreate {
		err = os.MkdirAll(filepath.Join(packageRoot, pathToCreate), DirMode)
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

	err = CopyFile(osquerydPath, filepath.Join(packageRoot, binaryDirectory, "osqueryd"))
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the osqueryd binary to the packaging root")
	}

	// The initial launcher (and extension) binary
	err = CopyFile(
		filepath.Join(LauncherSource(), "build/linux/launcher"),
		filepath.Join(packageRoot, binaryDirectory, "launcher"),
	)
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the launcher binary to the packaging root")
	}

	err = CopyFile(
		filepath.Join(LauncherSource(), "build/linux/osquery-extension.ext"),
		filepath.Join(packageRoot, binaryDirectory, "osquery-extension.ext"),
	)
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the osquery-extension binary to the packaging root")
	}

	// The secret which the user will use to authenticate to the cloud
	err = ioutil.WriteFile(filepath.Join(packageRoot, configurationDirectory, "secret"), []byte(secret), FileMode)
	if err != nil {
		return "", "", errors.Wrap(err, "could not write secret string to file for packaging")
	}

	// Finally, now that the final directory structure of the package is
	// represented, we can create the package
	currentVersion := version.Version().Version

	outputPathDir, err := ioutil.TempDir("/tmp", "packages_")
	if err != nil {
		return "", "", errors.Wrap(err, "could not create final output directory for package")
	}

	debOutputFilename := fmt.Sprintf("launcher-linux-%s.deb", currentVersion)
	debOutputPath := filepath.Join(outputPathDir, debOutputFilename)

	rpmOutputFilename := fmt.Sprintf("launcher-linux-%s.rpm", currentVersion)
	rpmOutputPath := filepath.Join(outputPathDir, rpmOutputFilename)

	// Create the packages
	debCmd := exec.Command(
		"docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/pkgroot", packageRoot),
		"-v", fmt.Sprintf("%s:/out", outputPathDir),
		"kolide/fpm",
		"fpm",
		"-s", "dir",
		"-t", "deb",
		"-n", "launcher",
		"-v", currentVersion,
		"-p", filepath.Join("/out", debOutputFilename),
		"/pkgroot=/",
	)
	if err := debCmd.Run(); err != nil {
		return "", "", errors.Wrap(err, "could not create deb package")
	}

	rpmCmd := exec.Command(
		"docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/pkgroot", packageRoot),
		"-v", fmt.Sprintf("%s:/out", outputPathDir),
		"kolide/fpm",
		"fpm",
		"-s", "dir",
		"-t", "rpm",
		"-n", "launcher",
		"-v", currentVersion,
		"-p", filepath.Join("/out", rpmOutputFilename),
		"/pkgroot=/",
	)
	if err := rpmCmd.Run(); err != nil {
		return "", "", errors.Wrap(err, "could not create rpm package")
	}

	return debOutputPath, rpmOutputPath, nil
}

func createMacPackage(osqueryVersion, hostname, secret, macPackageSigningKey string, insecure, insecureGrpc bool, identifier string) (string, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("/tmp", "createKolideMacPackageInTempDir.packageRoot")
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
		err = os.MkdirAll(filepath.Join(packageRoot, pathToCreate), DirMode)
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

	err = CopyFile(localOsquerydPath, filepath.Join(packageRoot, osquerydPath))
	if err != nil {
		return "", errors.Wrap(err, "could not copy the osqueryd binary to the packaging root")
	}

	// The initial launcher (and extension) binary
	err = CopyFile(
		filepath.Join(LauncherSource(), "build/darwin/launcher"),
		filepath.Join(packageRoot, launcherPath),
	)
	if err != nil {
		return "", errors.Wrap(err, "could not copy the launcher binary to the packaging root")
	}

	err = CopyFile(
		filepath.Join(LauncherSource(), "build/darwin/osquery-extension.ext"),
		filepath.Join(packageRoot, binaryDirectory, "osquery-extension.ext"),
	)
	if err != nil {
		return "", errors.Wrap(err, "could not copy the osquery-extension binary to the packaging root")
	}

	// The LaunchDaemon which will connect the launcher to the cloud
	launchDaemonFile, err := os.Create(filepath.Join(packageRoot, launchDaemonDirectory, fmt.Sprintf("%s.plist", launchDaemonName)))
	if err != nil {
		return "", errors.Wrap(err, "could not open the LaunchDaemon path for writing")
	}
	opts := &launchDaemonTemplateOptions{
		ServerHostname:   grpcServerForHostname(hostname),
		RootDirectory:    rootDirectory,
		LauncherPath:     launcherPath,
		OsquerydPath:     osquerydPath,
		LogDirectory:     logDirectory,
		SecretPath:       secretPath,
		LaunchDaemonName: launchDaemonName,
		Insecure:         insecure,
		InsecureGrpc:     insecureGrpc,
	}
	if err := renderLaunchDaemon(launchDaemonFile, opts); err != nil {
		return "", errors.Wrap(err, "could not write LaunchDeamon content to file")
	}

	// The secret which the user will use to authenticate to the server
	err = ioutil.WriteFile(filepath.Join(packageRoot, secretPath), []byte(secret), FileMode)
	if err != nil {
		return "", errors.Wrap(err, "could not write secret string to file for packaging")
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

	// Next, we calculate versions and file paths
	currentVersion := version.Version().Version

	outputPathDir, err := ioutil.TempDir("/tmp", "packaging_")
	outputPath := filepath.Join(outputPathDir, fmt.Sprintf("launcher-darwin-%s.pkg", currentVersion))
	if err != nil {
		return "", errors.Wrap(err, "could not create final output directory for package")
	}

	// Build the macOS package
	err = pkgbuild(
		packageRoot,
		scriptDir,
		launchDaemonName,
		currentVersion,
		macPackageSigningKey,
		outputPath,
	)
	if err != nil {
		return "", errors.Wrap(err, "could not create macOS package")
	}

	return outputPath, nil
}

// launchDaemonTemplateOptions is a struct which contains dynamic LaunchDaemon
// parameters that will be rendered into a template in renderLaunchDaemon
type launchDaemonTemplateOptions struct {
	ServerHostname   string
	RootDirectory    string
	LauncherPath     string
	OsquerydPath     string
	LogDirectory     string
	SecretPath       string
	LaunchDaemonName string
	InsecureGrpc     bool
	Insecure         bool
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
            <string>{{.OsquerydPath}}</string>
        </dict>
        <key>RunAtLoad</key>
        <true/>
        <key>KeepAlive</key>
        <true/>
        <key>ThrottleInterval</key>
        <integer>60</integer>
        <key>ProgramArguments</key>
        <array>
            <string>{{.LauncherPath}}</string>
            <string>--debug</string>
            {{if .InsecureGrpc}}<string>--insecure_grpc</string>{{end}}
            {{if .Insecure}}<string>--insecure</string>{{end}}
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
	return cmd.Run()
}

// grpcServerForHostname returns the gRPC server hostname given a web address
// that was serving the website itself
func grpcServerForHostname(hostname string) string {
	switch hostname {
	case "localhost:5000":
		return "localhost:8082"
	case "master.cloud.kolide.net":
		return "master-grpc.cloud.kolide.net:443"
	case "kolide.co", "kolide.com":
		return "launcher.kolide.com:443"
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

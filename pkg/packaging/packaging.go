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
func CreatePackages(po PackageOptions) (*PackagePaths, error) {
	macPkgDestinationPath, err := CreateMacPackage(po)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate macOS package")
	}

	debDestinationPath, rpmDestinationPath, err := CreateLinuxPackages(po)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate linux packages")
	}

	return &PackagePaths{
		MacOS: macPkgDestinationPath,
		Deb:   debDestinationPath,
		RPM:   rpmDestinationPath,
	}, nil
}

func createInitFiles(opts *ServiceOptions, serviceDirectory string, initFileName string, packageRoot string, binaryDirectory string, postInstallScript string, postInstallLauncherContents string, systemd bool) error {
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
		if err := opts.Render(initFile, SystemD); err != nil {
			return errors.Wrap(err, "could not render init system file")
		}
	} else {
		if err := opts.Render(initFile, Init); err != nil {
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

func CreateLinuxPackages(po PackageOptions) (string, string, error) {
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
	rootDirectory := filepath.Join("/var", po.Identifier, sanitizeHostname(po.Hostname))
	binaryDirectory := filepath.Join("/usr/local", po.Identifier, "bin")
	configurationDirectory := filepath.Join("/etc", po.Identifier)
	serviceDirectory := "/etc/systemd/system"

	if !po.Systemd {
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
	osquerydPath, err := FetchOsquerydBinary(po.CacheDir, po.OsqueryVersion, "linux")
	if err != nil {
		return "", "", errors.Wrap(err, "could not fetch path to osqueryd binary")
	}

	err = fs.CopyFile(osquerydPath, filepath.Join(packageRoot, binaryDirectory, "osqueryd"))
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the osqueryd binary to the packaging root")
	}

	// The secret which the user will use to authenticate to the cloud
	secretPath := filepath.Join(configurationDirectory, "secret")

	if !po.OmitSecret {
		err = ioutil.WriteFile(filepath.Join(packageRoot, secretPath), []byte(po.Secret), secretPerms)
		if err != nil {
			return "", "", errors.Wrap(err, "could not write secret string to file for packaging")
		}
	}

	var rootPEMPath string
	if po.RootPEM != "" {
		rootPEMPath = filepath.Join(configurationDirectory, "roots.pem")

		if err := fs.CopyFile(po.RootPEM, filepath.Join(packageRoot, rootPEMPath)); err != nil {
			return "", "", errors.Wrap(err, "copy root PEM")
		}
		if err := os.Chmod(filepath.Join(packageRoot, rootPEMPath), 0600); err != nil {
			return "", "", errors.Wrap(err, "chmod root PEM")
		}
	}

	if po.UpdateChannel == "" {
		po.UpdateChannel = "stable"
	}

	opts := &ServiceOptions{
		ServiceName:       "launcher",
		ServerHostname:    grpcServerForHostname(po.Hostname),
		RootDirectory:     rootDirectory,
		SecretPath:        secretPath,
		OsquerydPath:      filepath.Join(binaryDirectory, "osqueryd"),
		LauncherPath:      filepath.Join(binaryDirectory, "launcher"),
		Insecure:          po.Insecure,
		InsecureGrpc:      po.InsecureGrpc,
		Autoupdate:        po.Autoupdate,
		UpdateChannel:     po.UpdateChannel,
		Control:           po.Control,
		InitialRunner:     po.InitialRunner,
		ControlHostname:   po.ControlHostname,
		DisableControlTLS: po.DisableControlTLS,
		CertPins:          po.CertPins,
		RootPEM:           rootPEMPath,
	}

	if po.Systemd {
		initFileName := "launcher.service"
		postInstallLauncherContents := `#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable launcher
systemctl restart launcher`
		createInitFiles(opts, serviceDirectory, initFileName, packageRoot, binaryDirectory, postInstallScript, postInstallLauncherContents, po.Systemd)

	} else { //not systemd, so assume init
		initFileName := "launcher"
		// The post install step
		postInstallLauncherContents := `#!/bin/bash
sudo service launcher restart`
		createInitFiles(opts, serviceDirectory, initFileName, packageRoot, binaryDirectory, postInstallScript, postInstallLauncherContents, po.Systemd)
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

	if po.OutputPathDir == "" {
		po.OutputPathDir, err = ioutil.TempDir("/tmp", "packages_")
		if err != nil {
			return "", "", errors.Wrap(err, "could not create final output directory for package")
		}
	}

	if err = os.MkdirAll(po.OutputPathDir, 0755); err != nil {
		return "", "", errors.Wrapf(err, "could not create directory %s", po.OutputPathDir)
	}

	debOutputFilename := fmt.Sprintf("launcher-linux-%s.deb", po.PackageVersion)
	debOutputPath := filepath.Join(po.OutputPathDir, debOutputFilename)

	rpmOutputFilename := fmt.Sprintf("launcher-linux-%s.rpm", po.PackageVersion)
	rpmOutputPath := filepath.Join(po.OutputPathDir, rpmOutputFilename)

	// Create the packages
	containerPackageRoot := "/pkgroot"

	afterInstall := filepath.Join(containerPackageRoot, binaryDirectory, postInstallScript)

	debCmd := exec.Command(
		"docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:%s", packageRoot, containerPackageRoot),
		"-v", fmt.Sprintf("%s:/out", po.OutputPathDir),
		"kolide/fpm",
		"fpm",
		"-s", "dir",
		"-t", "deb",
		"-n", "launcher",
		"-v", po.PackageVersion,
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
		"-v", fmt.Sprintf("%s:/out", po.OutputPathDir),
		"kolide/fpm",
		"fpm",
		"-s", "dir",
		"-t", "rpm",
		"-n", "launcher",
		"-v", po.PackageVersion,
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

func CreateMacPackage(po PackageOptions) (string, error) {
	prep, err := prepare(LaunchD, "darwin", po)
	if err != nil {
		return "", errors.Wrap(err, "prepare")
	}
	if po.OutputPathDir == "" {
		po.OutputPathDir, err = ioutil.TempDir("/tmp", "packaging_")
		if err != nil {
			return "", errors.Wrap(err, "could not create final output directory for package")
		}
	}

	if err = os.MkdirAll(po.OutputPathDir, 0755); err != nil {
		return "", errors.Wrapf(err, "could not create directory %s", po.OutputPathDir)
	}

	outputPath := filepath.Join(po.OutputPathDir, fmt.Sprintf("launcher-darwin-%s.pkg", po.PackageVersion))

	// Build the macOS package
	err = pkgbuild(
		prep.PackageRoot,
		prep.ScriptsRoot,
		po.Identifier,
		po.PackageVersion,
		po.MacPackageSigningKey,
		outputPath,
	)
	if err != nil {
		return "", errors.Wrap(err, "could not create macOS package")
	}

	return outputPath, nil
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
		"--identifier", fmt.Sprintf("com.%s.launcher", identifier),
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

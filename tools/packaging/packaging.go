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

// CreatePackages will create a launcher macOS package given an upload root
// where the packages should be stores, a specific osquery version identifier,
// a munemo tenant identifier, and a key used to sign the enrollment secret JWT
// token. The output paths of the packages are returned and an error if the
// operation was not successful.
func CreatePackages(uploadRoot, osqueryVersion, hostname, tenant string, pemKey []byte, macPackageSigningKey string) (*PackagePaths, error) {
	macPkgDestinationPath, err := createMacPackage(uploadRoot, osqueryVersion, hostname, tenant, pemKey, macPackageSigningKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate macOS package")
	}

	debDestinationPath, rpmDestinationPath, err := createLinuxPackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate linux packages")
	}

	return &PackagePaths{
		MacOS: macPkgDestinationPath,
		Deb:   debDestinationPath,
		RPM:   rpmDestinationPath,
	}, nil
}

// sanitizeHostname will replace any ":" characters in a given hostname with "-"
// This is useful because ":" is not a valid character for file paths.
func sanitizeHostname(hostname string) string {
	return strings.Replace(hostname, ":", "-", -1)
}

func createLinuxPackages(uploadRoot, osqueryVersion, hostname, tenant string, pemKey []byte) (string, string, error) {
	debPath, rpmPath, err := createLinuxPackagesInTempDir(osqueryVersion, tenant, hostname, pemKey)
	if err != nil {
		return "", "", errors.Wrap(err, "could not make linux packages")
	}
	defer os.RemoveAll(filepath.Dir(debPath))
	defer os.RemoveAll(filepath.Dir(rpmPath))

	debRoot := filepath.Join(uploadRoot, sanitizeHostname(hostname), tenant, "ubuntu")
	if err := os.MkdirAll(debRoot, DirMode); err != nil {
		return "", "", errors.Wrap(err, "could not create deb root")
	}

	rpmRoot := filepath.Join(uploadRoot, sanitizeHostname(hostname), tenant, "centos")
	if err := os.MkdirAll(rpmRoot, DirMode); err != nil {
		return "", "", errors.Wrap(err, "could not create rpm root")
	}

	debDestinationPath := filepath.Join(debRoot, "launcher.deb")
	if err = CopyFile(debPath, debDestinationPath); err != nil {
		return "", "", errors.Wrap(err, "could not copy file to upload root")
	}

	rpmDestinationPath := filepath.Join(rpmRoot, "launcher.rpm")
	if err = CopyFile(rpmPath, rpmDestinationPath); err != nil {
		return "", "", errors.Wrap(err, "could not copy file to upload root")
	}
	return debDestinationPath, rpmDestinationPath, nil

}

func createMacPackage(uploadRoot, osqueryVersion, hostname, tenant string, pemKey []byte, macPackageSigningKey string) (string, error) {
	macPackagePath, err := createMacPackageInTempDir(osqueryVersion, tenant, hostname, pemKey, macPackageSigningKey)
	if err != nil {
		return "", errors.Wrap(err, "could not make macOS package")
	}
	defer os.RemoveAll(filepath.Dir(macPackagePath))

	darwinRoot := filepath.Join(uploadRoot, sanitizeHostname(hostname), tenant, "darwin")
	if err := os.MkdirAll(darwinRoot, DirMode); err != nil {
		return "", errors.Wrap(err, "could not create darwin root")
	}

	destinationPath := filepath.Join(darwinRoot, "launcher.pkg")
	if err = CopyFile(macPackagePath, destinationPath); err != nil {
		return "", errors.Wrap(err, "could not copy file to upload root")
	}
	return destinationPath, nil
}

// launchDaemonTemplateOptions is a struct which contains dynamic LaunchDaemon
// parameters that will be rendered into a template in renderLaunchDaemon
type launchDaemonTemplateOptions struct {
	KolideURL    string
	InsecureGrpc bool
}

// renderLaunchDaemon renders a LaunchDaemon to start and schedule the launcher.
func renderLaunchDaemon(w io.Writer, options *launchDaemonTemplateOptions) error {
	launchDaemonTemplate :=
		`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>com.kolide.launcher</string>
        <key>EnvironmentVariables</key>
        <dict>
            <key>KOLIDE_LAUNCHER_ROOT_DIRECTORY</key>
            <string>/var/kolide</string>
            <key>KOLIDE_LAUNCHER_KOLIDE_URL</key>
            <string>{{.KolideURL}}</string>
            <key>KOLIDE_LAUNCHER_ENROLL_SECRET_PATH</key>
            <string>/etc/kolide/secret</string>
        </dict>
        <key>RunAtLoad</key>
        <true/>
        <key>KeepAlive</key>
        <true/>
        <key>ThrottleInterval</key>
        <integer>60</integer>
        <key>ProgramArguments</key>
        <array>
            <string>/usr/local/kolide/bin/launcher</string>
            {{if .InsecureGrpc}}<string>--insecure_grpc</string>{{end}}
        </array>
        <key>StandardErrorPath</key>
        <string>/var/log/kolide/launcher-stderr.log</string>
        <key>StandardOutPath</key>
        <string>/var/log/kolide/launcher-stdout.log</string>
    </dict>
</plist>`
	t, err := template.New("LaunchDaemon").Parse(launchDaemonTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse LaunchDaemon template")
	}
	return t.ExecuteTemplate(w, "LaunchDaemon", options)
}

// pkgbuild runs the following pkgbuild command:
//   pkgbuild \
//     --root ${packageRoot} \
//     --scripts ${scriptsRoot} \
//     --identifier ${identifier} \
//     --version ${packageVersion} \
//     ${outputPath}
func pkgbuild(packageRoot, scriptsRoot, identifier, version, macPackageSigningKey, outputPath string) error {

	args := []string{"pkgbuild",
		"--root", packageRoot,
		"--scripts", scriptsRoot,
		"--identifier", identifier,
		"--version", version,
	}

	if macPackageSigningKey != "" {
		args = append(args, "--sign", macPackageSigningKey)
	}

	args = append(args, outputPath)
	cmd := exec.Command(strings.Join(args, " "))
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
		return fmt.Sprintf("%s:443", hostname)
	}
}

// createMacPackageInTempDir will create a launcher macOS package given a specific osquery
// version identifier, a munemo tenant identifier, and a key used to sign the
// enrollment secret JWT token. The output path of the package is returned and
// an error if the operation was not successful.
func createMacPackageInTempDir(osqueryVersion, tenantIdentifier, hostname string, pemKey []byte, macPackageSigningKey string) (string, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("/tmp", "createMacPackageInTempDir.packageRoot")
	if err != nil {
		return "", errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(packageRoot)

	// a macOS package is basically the ability to lay an additive addition of
	// files to the file system, as well as specify exeuctable scripts at certain
	// points in the installation process (before install, after, etc.).

	// Here, we must create the directory structure of our package.
	// First, we create all of the directories that we will need:
	pathsToCreate := []string{
		"/etc/kolide",
		"/var/kolide",
		"/var/log/kolide",
		"/usr/local/kolide/bin",
		"/Library/LaunchDaemons",
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
	osquerydPath, err := FetchOsquerydBinary(osqueryVersion, "darwin")
	if err != nil {
		return "", errors.Wrap(err, "could not fetch path to osqueryd binary")
	}

	err = CopyFile(osquerydPath, filepath.Join(packageRoot, "/usr/local/kolide/bin/osqueryd"))
	if err != nil {
		return "", errors.Wrap(err, "could not copy the osqueryd binary to the packaging root")
	}

	// The initial launcher (and extension) binary
	err = CopyFile(
		filepath.Join(LauncherSource(), "build/darwin/launcher"),
		filepath.Join(packageRoot, "/usr/local/kolide/bin/launcher"),
	)
	if err != nil {
		return "", errors.Wrap(err, "could not copy the launcher binary to the packaging root")
	}

	err = CopyFile(
		filepath.Join(LauncherSource(), "build/darwin/osquery-extension.ext"),
		filepath.Join(packageRoot, "/usr/local/kolide/bin/osquery-extension.ext"),
	)
	if err != nil {
		return "", errors.Wrap(err, "could not copy the osquery-extension binary to the packaging root")
	}

	// The LaunchDaemon which will connect the launcher to the cloud
	launchDaemonFile, err := os.Create(filepath.Join(packageRoot, "/Library/LaunchDaemons/com.kolide.launcher.plist"))
	if err != nil {
		return "", errors.Wrap(err, "could not open the LaunchDaemon path for writing")
	}
	opts := &launchDaemonTemplateOptions{
		KolideURL: grpcServerForHostname(hostname),
	}
	if hostname == "localhost:5000" {
		opts.InsecureGrpc = true
	}
	if err := renderLaunchDaemon(launchDaemonFile, opts); err != nil {
		return "", errors.Wrap(err, "could not write LaunchDeamon content to file")
	}

	// The secret which the user will use to authenticate to the cloud
	secretString, err := enrollSecret(tenantIdentifier, pemKey)
	if err != nil {
		return "", errors.Wrap(err, "could not generate secret for tenant")
	}
	err = ioutil.WriteFile(filepath.Join(packageRoot, "/etc/kolide/secret"), []byte(secretString), FileMode)
	if err != nil {
		return "", errors.Wrap(err, "could not write secret string to file for packaging")
	}

	// Finally, now that the final directory structure of the package is
	// represented, we can create the package
	currentVersion := version.Version().Version

	outputPathDir, err := ioutil.TempDir("/tmp", fmt.Sprintf("%s-%s-", sanitizeHostname(hostname), tenantIdentifier))
	outputPath := filepath.Join(outputPathDir, fmt.Sprintf("launcher-darwin-%s.pkg", currentVersion))
	if err != nil {
		return "", errors.Wrap(err, "could not create final output directory for package")
	}

	err = pkgbuild(
		packageRoot,
		filepath.Join(Gopath(), "src/github.com/kolide/launcher/tools/packaging/macos/scripts"),
		"com.kolide.launcher",
		currentVersion,
		macPackageSigningKey,
		outputPath,
	)
	if err != nil {
		return "", errors.Wrap(err, "could not create macOS package")
	}

	return outputPath, nil
}

// createLinuxPackagesInTempDir will create a deb and rpm package given a specific osquery
// version identifier, a munemo tenant identifier, and a key used to sign the
// enrollment secret JWT token. The output path of the package is returned and
// an error if the operation was not successful.
func createLinuxPackagesInTempDir(osqueryVersion, tenantIdentifier, hostname string, pemKey []byte) (string, string, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("/tmp", "createLinuxPackagesInTempDir.packageRoot")
	if err != nil {
		return "", "", errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(packageRoot)

	// Here, we must create the directory structure of our package.
	// First, we create all of the directories that we will need:
	pathsToCreate := []string{
		"/etc/kolide",
		"/var/kolide",
		"/var/log/kolide",
		"/usr/local/kolide/bin",
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

	err = CopyFile(osquerydPath, filepath.Join(packageRoot, "/usr/local/kolide/bin/osqueryd"))
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the osqueryd binary to the packaging root")
	}

	// The initial launcher (and extension) binary
	err = CopyFile(
		filepath.Join(LauncherSource(), "build/linux/launcher"),
		filepath.Join(packageRoot, "/usr/local/kolide/bin/launcher"),
	)
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the launcher binary to the packaging root")
	}

	err = CopyFile(
		filepath.Join(LauncherSource(), "build/linux/osquery-extension.ext"),
		filepath.Join(packageRoot, "/usr/local/kolide/bin/osquery-extension.ext"),
	)
	if err != nil {
		return "", "", errors.Wrap(err, "could not copy the osquery-extension binary to the packaging root")
	}

	// The secret which the user will use to authenticate to the cloud
	secretString, err := enrollSecret(tenantIdentifier, pemKey)
	if err != nil {
		return "", "", errors.Wrap(err, "could not generate secret for tenant")
	}
	err = ioutil.WriteFile(filepath.Join(packageRoot, "/etc/kolide/secret"), []byte(secretString), FileMode)
	if err != nil {
		return "", "", errors.Wrap(err, "could not write secret string to file for packaging")
	}

	// Finally, now that the final directory structure of the package is
	// represented, we can create the package
	currentVersion := version.Version().Version

	outputPathDir, err := ioutil.TempDir("/tmp", fmt.Sprintf("%s-%s-", sanitizeHostname(hostname), tenantIdentifier))
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

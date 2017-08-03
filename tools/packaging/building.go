package packaging

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/kolide/kit/version"
	"github.com/pkg/errors"
)

// launchDaemonTemplateOptions is a struct which contains dynamic LaunchDaemon
// parameters that will be rendered into a template in renderLaunchDaemon
type launchDaemonTemplateOptions struct {
	KolideURL string
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
            <key>KOLIDE_LAUNCHER_WORKING_DIRECTORY</key>
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
func pkgbuild(packageRoot, scriptsRoot, identifier, version, outputPath string) error {
	cmd := exec.Command("pkgbuild",
		"--root", packageRoot,
		"--scripts", scriptsRoot,
		"--identifier", identifier,
		"--version", version,
		outputPath,
	)
	return cmd.Run()
}

// MakeMacOSPkg will create a launcher macOS package given a specific osquery
// version identifier, a munemo tenant identifier, and a key used to sign the
// enrollment secret JWT token. The output path of the package is returned and
// an error if the operation was not successful.
func MakeMacOSPkg(osqueryVersion, tenantIdentifier, hostname string, pemKey []byte) (string, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("", "MakeMacOSPkg.packageRoot")
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
			return "", errors.Wrap(err, fmt.Sprintf("could not make directory %s/%s", packageRoot, pathToCreate))
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
	err = renderLaunchDaemon(launchDaemonFile, &launchDaemonTemplateOptions{
		KolideURL: fmt.Sprintf("https://%s", hostname),
	})
	if err != nil {
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

	outputPathDir, err := ioutil.TempDir("", fmt.Sprintf("%s-%s-", hostname, tenantIdentifier))
	outputPath := filepath.Join(outputPathDir, fmt.Sprintf("launcher-darwin-%s.pkg", currentVersion))
	if err != nil {
		return "", errors.Wrap(err, "could not create final output directory for package")
	}

	err = pkgbuild(
		packageRoot,
		fmt.Sprintf("%s/src/github.com/kolide/launcher/tools/packaging/macos/scripts", Gopath()),
		"com.kolide.launcher",
		currentVersion,
		outputPath,
	)
	if err != nil {
		return "", errors.Wrap(err, "could not create macOS package")
	}

	return outputPath, nil
}

// MakeLinuxPackages will create a deb and rpm package given a specific osquery
// version identifier, a munemo tenant identifier, and a key used to sign the
// enrollment secret JWT token. The output path of the package is returned and
// an error if the operation was not successful.
func MakeLinuxPackages(osqueryVersion, tenantIdentifier, hostname string, pemKey []byte) (string, string, error) {
	return "", "", errors.New("unimplemented")
}

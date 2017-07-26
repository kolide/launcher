// +build darwin

package packaging

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"text/template"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/pkg/errors"
)

func Gopath() string {
	home := env.String("HOME", "~/")
	return env.String("GOPATH", fmt.Sprintf("%s/go", home))
}

type launchDaemonTemplateOptions struct {
	KolideURL string
}

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

// Pkgbuild runs the following pkgbuild command:
//   pkgbuild \
//     --root ${packageRoot} \
//     --scripts ${scriptsRoot} \
//     --identifier ${identifier} \
//     --version ${packageVersion} \
//     ${outputPath}
func Pkgbuild(packageRoot, scriptsRoot, identifier, version, outputPath string) error {
	cmd := exec.Command("pkgbuild",
		"--root", packageRoot,
		"--scripts", scriptsRoot,
		"--identifier", identifier,
		"--version", version,
		outputPath,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// MakeMacOSPkg will create a launcher macOS package given a specific osquery
// verison identifier, a munemo tenant identifier, and a key used to sign the
// enrollment secret JWT token. The output path of the package is returned and
// an error if the operation was not successful.
func MakeMacOSPkg(osqueryVersion, tenantIdentifier string, pemKey []byte) (string, error) {
	// first, we have to create a local temp directory on disk that we will use as
	// a packaging root, but will delete once the generated package is created and
	// stored on disk
	packageRoot, err := ioutil.TempDir("", "MakeMacOSPkg.packageRoot")
	if err != nil {
		return "", errors.Wrap(err, "unable to create temporary packaging root directory")
	}
	defer os.RemoveAll(packageRoot)
	_ = packageRoot

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
	_ = pathsToCreate

	// Next we create each file that gets laid down as a result of the package
	// installation:

	// The initial osqueryd binary
	// TODO

	// The initial launcher (and extension) binary
	// TODO

	// The LaunchDaemon which will connect the launcher to the cloud
	// TODO

	// The secret which the user will use to authenticate to the cloud
	// TODO

	// Finally, now that the final directory structure of the package is
	// represented, we can create the package
	currentVersion := version.Version().Version

	outputPathDir, err := ioutil.TempDir("", "MakeMacOSPkg.outputPath")
	outputPath := fmt.Sprintf("%s/launcher-darwin-%s.pkg", outputPathDir, currentVersion)
	if err != nil {
		return "", errors.Wrap(err, "could not create final output directory for package")
	}

	err = Pkgbuild(
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

// +build darwin

package packaging

import (
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

var launchDaemonTemplate = `
<?xml version="1.0" encoding="UTF-8"?>
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
</plist>
`

type launchDaemonTemplateOptions struct {
	KolideURL string
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
	pathsToCreate := []string{
		"/etc/kolide",
		"/var/kolide",
		"/var/log/kolide",
		"/usr/local/kolide/bin",
		"/Library/LaunchDaemons",
	}
	_ = pathsToCreate
	return "", errors.New("not implemented")
}

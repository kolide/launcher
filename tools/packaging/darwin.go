// +build darwin

package packaging

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

// Pkgbuild runs the following pkgbuild command:
//   pkgbuild \
//     --root ${packageRoot} \
//     --scripts ${scriptsRoot} \
//     --identifier ${packageID} \
//     --version ${packageVersion} \
//     build/${packageName}-${packageVersion}.pkg
func Pkgbuild(packageRoot, scriptsRoot, version, packageName string) error {
	identifier := "com.kolide.osquery"
	cmd := exec.Command("pkgbuild",
		"--root", packageRoot,
		"--scripts", scriptsRoot,
		"--identifier", identifier,
		"--version", version,
		fmt.Sprintf("build/%s", packageName),
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// MakeMacOSPkg will create a launcher macOS package given a specific launcher
// and osquery verison identifier, a munemo tenant identifier, and a key used to
// sign the enrollment secret JWT token. The output path of the package is
// returned and an error if the operation was not successful.
func MakeMacOSPkg(launcherVersion, osqueryVersion, tenantIdentifier string, pemKey []byte) (string, error) {
	return "", errors.New("not implemented")
}

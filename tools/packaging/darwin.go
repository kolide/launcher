// +build darwin

package packaging

import (
	"fmt"
	"os"
	"os/exec"
)

/*
runs the following pkgbuild command:
  pkgbuild \
  --root ${packageRoot} \
  --scripts ${scriptsRoot} \
  --identifier ${packageID} \
  --version ${packageVersion} \
  build/${packageName}-${packageVersion}.pkg
*/
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

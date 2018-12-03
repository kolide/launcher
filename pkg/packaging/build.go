package packaging

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

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

func linuxbuild(kind, packageRoot, scriptsRoot, version, outputdir, outputpath string) error {
	containerPackageRoot := "/pkgroot"
	cmd := exec.Command(
		"docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:%s", packageRoot, containerPackageRoot),
		"-v", fmt.Sprintf("%s:/out", outputdir),
		"kolide/fpm",
		"fpm",
		"-s", "dir",
		"-t", kind, // deb or rpm
		"-n", "launcher",
		"-v", version,
		"-p", filepath.Join("/out", outputpath),
		"--after-install", filepath.Join(scriptsRoot, "postinstall"),
		"-C", containerPackageRoot,
		".",
	)
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		io.Copy(os.Stderr, stderr)
		return errors.Wrapf(err, "could not create %s package", kind)
	}
	return nil
}

package packageTNG

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

func PackageRPM(w io.Writer, po *PackageOptions) error {
	if err := isDirectory(po.Root); err != nil {
		return err
	}

	outputFilename := fmt.Sprintf("%s-%s.rpm", po.Name, po.Version)

	outputPathDir, err := ioutil.TempDir("/tmp", "packaging-rpm-output")
	if err != nil {
		return errors.Wrap(err, "making TempDir")
	}
	defer os.RemoveAll(outputPathDir)

	fpmCommand := []string{
		"fpm",
		"-s", "dir",
		"-t", "rpm",
		"-n", po.Name,
		"-v", po.Version,
		"-p", filepath.Join("/out", outputFilename),
		"-C", "/pkgsrc",
	}

	if po.AfterInstall != "" {
		fpmCommand = append(fpmCommand, "--after-install", po.AfterInstall)
	}

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/pkgsrc", po.Root),
		"-v", fmt.Sprintf("%s:/out", outputPathDir),
		"kolide/fpm",
	}

	cmd := exec.Command("docker", append(dockerArgs, fpmCommand...)...)

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "creating rpm package: %s", stderr)
	}

	outputFH, err := os.Open(filepath.Join(outputPathDir, outputFilename))
	if err != nil {
		return errors.Wrap(err, "opening resultant output file")
	}

	if _, err := io.Copy(w, outputFH); err != nil {
		return errors.Wrap(err, "copying output")
	}

	return nil
}

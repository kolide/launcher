package packaging

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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

func CreateLinuxPackages(po PackageOptions) (string, string, error) {
	var flavor Flavor
	if po.Systemd {
		flavor = SystemD
	} else {
		flavor = Init
	}
	prep, err := prepare(flavor, "linux", po)
	if err != nil {
		return "", "", errors.Wrap(err, "prepare")
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

	if err := linuxbuild(
		"deb",
		prep.PackageRoot,
		prep.ScriptsRoot,
		po.PackageVersion,
		po.OutputPathDir,
		debOutputPath,
	); err != nil {
		return "", "", err
	}

	rpmOutputFilename := fmt.Sprintf("launcher-linux-%s.rpm", po.PackageVersion)
	rpmOutputPath := filepath.Join(po.OutputPathDir, rpmOutputFilename)
	if err := linuxbuild(
		"rpm",
		prep.PackageRoot,
		prep.ScriptsRoot,
		po.PackageVersion,
		po.OutputPathDir,
		rpmOutputPath,
	); err != nil {
		return "", "", err
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

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/launcher/tools/packaging"
	"github.com/pkg/errors"
)

func safePathHostname(hostname string) string {
	return strings.Replace(hostname, ":", "-", -1)
}

func createPackages(logger log.Logger, uploadRoot, osqueryVersion, hostname, tenant string, pemKey []byte) error {
	macPkgDestinationPath, err := createMacPackage(uploadRoot, osqueryVersion, hostname, tenant, pemKey)
	if err != nil {
		return errors.Wrap(err, "could not generate macOS package")
	}
	level.Debug(logger).Log(
		"msg", "created macOS package",
		"path", macPkgDestinationPath,
		"tenant", tenant,
		"hostname", hostname,
	)

	debDestinationPath, rpmDestinationPath, err := createLinuxPackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey)
	if err != nil {
		return errors.Wrap(err, "could not generate linux packages")
	}
	level.Debug(logger).Log(
		"msg", "created linux packages",
		"deb", debDestinationPath,
		"rpm", rpmDestinationPath,
		"tenant", tenant,
		"hostname", hostname,
	)

	return nil
}

func createLinuxPackages(uploadRoot, osqueryVersion, hostname, tenant string, pemKey []byte) (string, string, error) {
	debPath, rpmPath, err := packaging.MakeLinuxPackages(osqueryVersion, tenant, hostname, pemKey)
	if err != nil {
		return "", "", errors.Wrap(err, "could not make macOS package")
	}
	defer os.RemoveAll(filepath.Dir(debPath))
	defer os.RemoveAll(filepath.Dir(rpmPath))

	debRoot := filepath.Join(uploadRoot, safePathHostname(hostname), tenant, "deb")
	if err := os.MkdirAll(debRoot, packaging.DirMode); err != nil {
		return "", "", errors.Wrap(err, "could not create deb root")
	}

	rpmRoot := filepath.Join(uploadRoot, safePathHostname(hostname), tenant, "rpm")
	if err := os.MkdirAll(debRoot, packaging.DirMode); err != nil {
		return "", "", errors.Wrap(err, "could not create rpm root")
	}

	debDestinationPath := filepath.Join(debRoot, "launcher.deb")
	if err = packaging.CopyFile(debPath, debDestinationPath); err != nil {
		return "", "", errors.Wrap(err, "could not copy file to upload root")
	}

	rpmDestinationPath := filepath.Join(rpmRoot, "launcher.rpm")
	if err = packaging.CopyFile(rpmPath, rpmDestinationPath); err != nil {
		return "", "", errors.Wrap(err, "could not copy file to upload root")
	}
	return debDestinationPath, rpmDestinationPath, nil

}

func createMacPackage(uploadRoot, osqueryVersion, hostname, tenant string, pemKey []byte) (string, error) {
	macPackagePath, err := packaging.MakeMacOSPkg(osqueryVersion, tenant, hostname, pemKey)
	defer os.RemoveAll(filepath.Dir(macPackagePath))
	if err != nil {
		return "", errors.Wrap(err, "could not make macOS package")
	}

	darwinRoot := filepath.Join(uploadRoot, safePathHostname(hostname), tenant, "darwin")
	if err := os.MkdirAll(darwinRoot, packaging.DirMode); err != nil {
		return "", errors.Wrap(err, "could not create darwin root")
	}

	destinationPath := filepath.Join(darwinRoot, "launcher.pkg")
	if err = packaging.CopyFile(macPackagePath, destinationPath); err != nil {
		return "", errors.Wrap(err, "could not copy file to upload root")
	}
	return destinationPath, nil
}

func runDev(args []string) error {
	flagset := flag.NewFlagSet("query", flag.ExitOnError)
	var (
		flDebug = flagset.Bool(
			"debug",
			false,
			"enable debug logging",
		)
		flOsqueryVersion = flagset.String(
			"osquery_version",
			env.String("KOLIDE_LAUNCHER_PACKAGE_BUILDER_OSQUERY_VERSION", ""),
			"the osquery version to include in the resultant packages",
		)
		flEnrollmentSecretSigningKeyPath = flagset.String(
			"enrollment_secret_signing_key",
			env.String("KOLIDE_LAUNCHER_PACKAGE_BUILDER_ENROLLMENT_SECRET_SIGNING_KEY", ""),
			"the path to the PEM key which is used to sign the enrollment secret JWT token",
		)
	)

	flagset.Usage = usageFor(flagset, "package-builder dev [flags]")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	if *flDebug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	osqueryVersion := *flOsqueryVersion
	if osqueryVersion == "" {
		osqueryVersion = "stable"
	}

	enrollmentSecretSigningKeyPath := *flEnrollmentSecretSigningKeyPath
	if enrollmentSecretSigningKeyPath == "" {
		enrollmentSecretSigningKeyPath = filepath.Join(packaging.LauncherSource(), "/tools/packaging/example_rsa.pem")
	}

	if _, err := os.Stat(enrollmentSecretSigningKeyPath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrap(err, "key file doesn't exist")
		} else {
			return errors.Wrap(err, "could not stat key file")
		}
	}
	pemKey, err := ioutil.ReadFile(enrollmentSecretSigningKeyPath)
	if err != nil {
		return errors.Wrap(err, "could not read the supplied key file")
	}

	// Generate packages for PRs
	prToStartFrom, prToGenerateUntil := 350, 400
	firstID, numberOfIDsToGenerate := 100001, 3

	uploadRoot, err := ioutil.TempDir("", "upload_")
	if err != nil {
		return errors.Wrap(err, "could not create upload root temporary directory")
	}
	defer os.RemoveAll(uploadRoot)

	makeHostnameDirInRoot := func(hostname string) error {
		if err := os.MkdirAll(filepath.Join(uploadRoot, safePathHostname(hostname)), packaging.DirMode); err != nil {
			return errors.Wrap(err, "could not create hostname root")
		}
		return nil
	}

	// Generate packages for localhost and master
	hostnames := []string{
		"master.cloud.kolide.net",
		"localhost:5000",
	}
	for _, hostname := range hostnames {
		if err := makeHostnameDirInRoot(hostname); err != nil {
			return err
		}
		for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.Munemo(id)
			if err := createPackages(logger, uploadRoot, osqueryVersion, hostname, tenant, pemKey); err != nil {
				return errors.Wrap(err, "could not generate macOS package for tenant")
			}
		}
	}

	// Generate packages for PRs
	for i := prToStartFrom; i <= prToGenerateUntil; i++ {
		hostname := fmt.Sprintf("%d.cloud.kolide.net", i)
		if err := makeHostnameDirInRoot(hostname); err != nil {
			return err
		}

		for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.Munemo(id)
			if err := createPackages(logger, uploadRoot, osqueryVersion, hostname, tenant, pemKey); err != nil {
				return errors.Wrap(err, "could not generate macOS package for tenant")
			}
		}
	}

	logger.Log(
		"msg", "package generation complete",
		"path", uploadRoot,
	)

	if err := packaging.GsutilRsync(uploadRoot, "gs://packaging/"); err != nil {
		return errors.Wrap(err, "could not upload files to GCS")
	}

	if err := os.RemoveAll(uploadRoot); err != nil {
		return errors.Wrap(err, "could not remove the upload root")
	}

	logger.Log("msg", "upload complete")

	return nil
}

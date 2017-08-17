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

func runDev(args []string) error {
	flagset := flag.NewFlagSet("dev", flag.ExitOnError)
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
	prToStartFrom, prToGenerateUntil := 445, 500
	firstID, numberOfIDsToGenerate := 100001, 1

	uploadRoot, err := ioutil.TempDir("", "upload_")
	if err != nil {
		return errors.Wrap(err, "could not create upload root temporary directory")
	}
	defer os.RemoveAll(uploadRoot)

	makeHostnameDirInRoot := func(hostname string) error {
		if err := os.MkdirAll(filepath.Join(uploadRoot, strings.Replace(hostname, ":", "-", -1)), packaging.DirMode); err != nil {
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
			tenant := packaging.TenantName(id)
			paths, err := packaging.CreatePackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey)
			if err != nil {
				return errors.Wrap(err, "could not generate package for tenant")
			}
			level.Debug(logger).Log(
				"msg", "created packages",
				"deb", paths.Deb,
				"rpm", paths.RPM,
				"mac", paths.MacOS,
				"tenant", tenant,
				"hostname", hostname,
			)
		}
	}

	// Generate packages for PRs
	for i := prToStartFrom; i <= prToGenerateUntil; i++ {
		hostname := fmt.Sprintf("%d.cloud.kolide.net", i)
		if err := makeHostnameDirInRoot(hostname); err != nil {
			return err
		}

		for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.TenantName(id)
			paths, err := packaging.CreatePackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey)
			if err != nil {
				return errors.Wrap(err, "could not generate package for tenant")
			}
			level.Debug(logger).Log(
				"msg", "created packages",
				"deb", paths.Deb,
				"rpm", paths.RPM,
				"mac", paths.MacOS,
				"tenant", tenant,
				"hostname", hostname,
			)
		}
	}

	logger.Log(
		"msg", "package generation complete",
		"path", uploadRoot,
	)

	if err := packaging.GsutilRsync(uploadRoot, "gs://kolide-ose-testing_packaging/"); err != nil {
		return errors.Wrap(err, "could not upload files to GCS")
	}

	if err := os.RemoveAll(uploadRoot); err != nil {
		return errors.Wrap(err, "could not remove the upload root")
	}

	logger.Log("msg", "upload complete")

	return nil
}

func runProd(args []string) error {
	flagset := flag.NewFlagSet("prod", flag.ExitOnError)
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

	flagset.Usage = usageFor(flagset, "package-builder prod [flags]")
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

	firstID, numberOfIDsToGenerate := 100001, 3

	uploadRoot, err := ioutil.TempDir("", "upload_")
	if err != nil {
		return errors.Wrap(err, "could not create upload root temporary directory")
	}
	defer os.RemoveAll(uploadRoot)

	// Generate packages for localhost and master
	hostnames := []string{
		"kolide.com",
	}
	for _, hostname := range hostnames {
		if err := os.MkdirAll(filepath.Join(uploadRoot, strings.Replace(hostname, ":", "-", -1)), packaging.DirMode); err != nil {
			return err
		}
		for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.TenantName(id)
			paths, err := packaging.CreatePackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey)
			if err != nil {
				return errors.Wrap(err, "could not generate package for tenant")
			}
			level.Debug(logger).Log(
				"msg", "created packages",
				"deb", paths.Deb,
				"rpm", paths.RPM,
				"mac", paths.MacOS,
				"tenant", tenant,
				"hostname", hostname,
			)
		}
	}

	if err := packaging.GsutilRsync(uploadRoot, "gs://kolide-website_packaging/"); err != nil {
		return errors.Wrap(err, "could not upload files to GCS")
	}

	if err := os.RemoveAll(uploadRoot); err != nil {
		return errors.Wrap(err, "could not remove the upload root")
	}

	logger.Log("msg", "upload complete")

	return nil
}

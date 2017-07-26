package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/tools/packaging"
)

// options is the set of configurable options that may be set when launching
// this program
type options struct {
	printVersion                   bool
	osqueryVersion                 string
	enrollmentSecretSigningKeyPath string
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
		flVersion = flag.Bool(
			"version",
			false,
			"print package-builder version and exit",
		)
		flOsqueryVersion = flag.String(
			"osquery_version",
			env.String("KOLIDE_LAUNCHER_PACKAGE_BUILDER_OSQUERY_VERSION", ""),
			"the osquery version to include in the resultant packages",
		)
		flEnrollmentSecretSigningKeyPath = flag.String(
			"enrollment_secret_signing_key",
			env.String("KOLIDE_LAUNCHER_PACKAGE_BUILDER_ENROLLMENT_SECRET_SIGNING_KEY", ""),
			"the path to the PEM key which is used to sign the enrollment secret JWT token",
		)
	)
	flag.Parse()

	opts := &options{
		osqueryVersion:                 *flOsqueryVersion,
		enrollmentSecretSigningKeyPath: *flEnrollmentSecretSigningKeyPath,
		printVersion:                   *flVersion,
	}

	if opts.osqueryVersion == "" {
		opts.osqueryVersion = "stable"
	}

	return opts, nil
}

func main() {
	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	opts, err := parseOptions()
	if err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not parse options: %s", err))
		os.Exit(1)
	}

	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	if opts.enrollmentSecretSigningKeyPath == "" {
		level.Warn(logger).Log("warning", "an enrollment secret signing key path was not specified, trying the test key")
		opts.enrollmentSecretSigningKeyPath = fmt.Sprintf("%s/tools/packaging/example_rsa.pem", packaging.LauncherSource())
	}

	if _, err := os.Stat(opts.enrollmentSecretSigningKeyPath); err != nil {
		if os.IsNotExist(err) {
			level.Error(logger).Log(
				"error", "Key file doesn't exist",
				"path", opts.enrollmentSecretSigningKeyPath,
			)
		} else {
			level.Error(logger).Log(
				"error", "Could not stat key file",
				"path", opts.enrollmentSecretSigningKeyPath,
				"message", err.Error(),
			)
		}
	}

	level.Debug(logger).Log(
		"osquery_version", opts.osqueryVersion,
		"enrollment_secret_signing_key", opts.enrollmentSecretSigningKeyPath,
		"message", "finished parsing arguments",
	)

	// Generate packages for PRs
	pemKey, err := ioutil.ReadFile(opts.enrollmentSecretSigningKeyPath)
	if err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not read the supplied key file: %s", err))
		os.Exit(1)
	}

	prToStartFrom := 350
	prToGenerateUntil := 500

	for i := prToStartFrom; i < prToGenerateUntil; i++ {
		hostname := fmt.Sprintf("%d.cloud.kolide.net", i)

		firstID := 100001
		numberOfIDsToGenerate := 2
		for id := firstID; id <= firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.Munemo(id)

			macPackagePath, err := packaging.MakeMacOSPkg(opts.osqueryVersion, tenant, hostname, pemKey)
			if err != nil {
				level.Error(logger).Log("error", fmt.Sprintf("Could not generate macOS package for tenant (%s): %s", tenant, err))
				os.Exit(1)
			}

			level.Debug(logger).Log(
				"msg", "Generated macOS package",
				"path", macPackagePath,
			)

			/*
				if err := packaging.UploadMacOSPkgToGCS(macPackagePath, tenant); err != nil {
					level.Error(logger).Log("error", fmt.Sprintf("Could not upload macOS package to GCS: %s", err))
				}

				logger.Log(
					"msg", "Successfully uploaded macOS package to GSC",
					"path", macPackagePath,
					"tenant", tenant,
				)
			*/
		}
	}

}

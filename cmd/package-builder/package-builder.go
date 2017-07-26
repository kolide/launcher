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
	logger := log.NewLogfmtLogger(os.Stderr)
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
		level.Error(logger).Log("error", "an enrollment secret signing key path was not specified")
		os.Exit(1)
	}

	level.Debug(logger).Log(
		"osquery_version", opts.osqueryVersion,
		"enrollment_secret_signing_key", opts.enrollmentSecretSigningKeyPath,
		"message", "finished parsing arguments",
	)

	pemKey, err := ioutil.ReadFile(opts.enrollmentSecretSigningKeyPath)
	if err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not read the supplied key file: %s", err))
		os.Exit(1)
	}

	firstID := 100001
	numberOfIDsToGenerate := 1000

	for id := firstID; id <= firstID+numberOfIDsToGenerate; id++ {
		tenant := packaging.Munemo(id)

		macPackagePath, err := packaging.MakeMacOSPkg(opts.osqueryVersion, tenant, pemKey)
		if err != nil {
			level.Error(logger).Log("error", fmt.Sprintf("Could not generate macOS package for tenant (%s): %s", tenant, err))
			os.Exit(1)
		}

		level.Debug(logger).Log(
			"msg", "Generated macOS package",
			"path", macPackagePath,
		)

		if err := packaging.UploadMacOSPkgToGCS(macPackagePath, tenant); err != nil {
			level.Error(logger).Log("error", fmt.Sprintf("Could not upload macOS package to GCS: %s", err))
		}

		logger.Log(
			"msg", "Successfully uploaded macOS package to GSC",
			"path", macPackagePath,
			"tenant", tenant,
		)
	}
}

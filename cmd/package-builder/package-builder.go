package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
	debugLogging                   bool
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
		flDebug = flag.Bool(
			"debug",
			false,
			"enable debug logging",
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
		debugLogging:                   *flDebug,
	}

	if opts.osqueryVersion == "" {
		opts.osqueryVersion = "stable"
	}

	if opts.enrollmentSecretSigningKeyPath == "" {
		opts.enrollmentSecretSigningKeyPath = fmt.Sprintf("%s/tools/packaging/example_rsa.pem", packaging.LauncherSource())
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

	if opts.debugLogging {
		level.Debug(logger).Log(
			"osquery_version", opts.osqueryVersion,
			"enrollment_secret_signing_key", opts.enrollmentSecretSigningKeyPath,
			"message", "finished parsing arguments",
		)
	}

	// Generate packages for PRs
	pemKey, err := ioutil.ReadFile(opts.enrollmentSecretSigningKeyPath)
	if err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not read the supplied key file: %s", err))
		os.Exit(1)
	}

	prToStartFrom := 350
	prToGenerateUntil := 400

	uploadRoot, err := ioutil.TempDir("", "upload_")
	if err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not create upload root temporary directory: %s", err))
	}

	for i := prToStartFrom; i < prToGenerateUntil; i++ {
		hostname := fmt.Sprintf("%d.cloud.kolide.net", i)

		if err := os.MkdirAll(filepath.Join(uploadRoot, hostname), packaging.DirMode); err != nil {
			level.Error(logger).Log("error", fmt.Sprintf("Could not create hostname root: %s", err))
		}

		firstID := 100001
		numberOfIDsToGenerate := 3
		for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.Munemo(id)

			macPackagePath, err := packaging.MakeMacOSPkg(opts.osqueryVersion, tenant, hostname, pemKey)
			if err != nil {
				level.Error(logger).Log("error", fmt.Sprintf("Could not generate macOS package for tenant (%s): %s", tenant, err))
				os.Exit(1)
			}

			darwinRoot := filepath.Join(uploadRoot, hostname, tenant, "darwin")
			if err := os.MkdirAll(darwinRoot, packaging.DirMode); err != nil {
				level.Error(logger).Log("error", fmt.Sprintf("Could not create darwin root: %s", err))
			}

			destinationPath := filepath.Join(uploadRoot, hostname, tenant, "darwin", "launcher.pkg")
			err = packaging.CopyFile(macPackagePath, destinationPath)
			if err != nil {
				level.Error(logger).Log("error", "Could not copy file from %s to %s: %s", macPackagePath, destinationPath, err)
				os.Exit(1)
			}

			if opts.debugLogging {
				level.Debug(logger).Log(
					"msg", "Copied macOS package for tenant and hostname",
					"source", macPackagePath,
					"destination", destinationPath,
					"tenant", tenant,
					"hostname", hostname,
				)
			}

			if err := os.RemoveAll(filepath.Dir(macPackagePath)); err != nil {
				level.Error(logger).Log("error", fmt.Sprintf("Could not remove the macOS package: %s", err))
				os.Exit(1)
			}
		}
	}

	level.Info(logger).Log(
		"msg", "package generation complete",
		"path", uploadRoot,
	)

	if err := packaging.GsutilRsync(uploadRoot, "gs://packaging/"); err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not upload files to GCS: %s", err))
		os.Exit(1)
	}

	if err := os.RemoveAll(uploadRoot); err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not remove the upload root: %s", err))
		os.Exit(1)
	}

	level.Info(logger).Log("msg", "upload complete")
}

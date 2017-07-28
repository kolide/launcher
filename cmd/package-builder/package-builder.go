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
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/tools/packaging"
	"github.com/pkg/errors"
)

// options is the set of configurable options that may be set when launching
// this program
type options struct {
	printVersion                   bool
	debugLogging                   bool
	osqueryVersion                 string
	enrollmentSecretSigningKeyPath string
}

func (opts *options) check() error {
	if _, err := os.Stat(opts.enrollmentSecretSigningKeyPath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrap(err, "key file doesn't exist")
		} else {
			return errors.Wrap(err, "could not stat key file")
		}
	}
	return nil
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

func safePathHostname(hostname string) string {
	return strings.Replace(hostname, ":", "-", -1)
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

func main() {
	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	opts, err := parseOptions()
	if err != nil {
		logger.Log(
			"msg", "could not parse options",
			"err", err,
		)
		os.Exit(1)
	}

	if opts.debugLogging {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	if err := opts.check(); err != nil {
		logger.Log(
			"msg", "invalid options",
			"err", err,
		)
		os.Exit(1)
	}

	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	level.Debug(logger).Log(
		"osquery_version", opts.osqueryVersion,
		"enrollment_secret_signing_key", opts.enrollmentSecretSigningKeyPath,
		"msg", "finished parsing arguments",
	)

	// Generate packages for PRs
	pemKey, err := ioutil.ReadFile(opts.enrollmentSecretSigningKeyPath)
	if err != nil {
		logger.Log(
			"msg", "could not read the supplied key file",
			"err", err,
		)
		os.Exit(1)
	}

	prToStartFrom, prToGenerateUntil := 350, 400
	firstID, numberOfIDsToGenerate := 100001, 3

	uploadRoot, err := ioutil.TempDir("", "upload_")
	if err != nil {
		logger.Log(
			"msg", "could not create upload root temporary directory",
			"err", err,
		)
		os.Exit(1)
	}
	defer os.RemoveAll(uploadRoot)

	makeHostnameDirInRoot := func(hostname string) {
		if err := os.MkdirAll(filepath.Join(uploadRoot, safePathHostname(hostname)), packaging.DirMode); err != nil {
			logger.Log(
				"msg", "could not create hostname root",
				"err", err,
			)
			os.Exit(1)
		}
	}

	// Generate packages for localhost and master
	hostnames := []string{
		"master.cloud.kolide.net",
		"localhost:5000",
	}
	for _, hostname := range hostnames {
		makeHostnameDirInRoot(hostname)
		for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.Munemo(id)
			destinationPath, err := createMacPackage(uploadRoot, opts.osqueryVersion, hostname, tenant, pemKey)
			if err != nil {
				logger.Log(
					"msg", "could not generate macOS package for tenant",
					"tenant", tenant,
					"err", err,
				)
				os.Exit(1)
			}
			level.Debug(logger).Log(
				"msg", "copied macOS package for tenant and hostname",
				"destination", destinationPath,
				"tenant", tenant,
				"hostname", hostname,
			)
		}
	}

	// Generate packages for PRs
	for i := prToStartFrom; i <= prToGenerateUntil; i++ {
		hostname := fmt.Sprintf("%d.cloud.kolide.net", i)
		makeHostnameDirInRoot(hostname)

		for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
			tenant := packaging.Munemo(id)
			destinationPath, err := createMacPackage(uploadRoot, opts.osqueryVersion, hostname, tenant, pemKey)
			if err != nil {
				logger.Log(
					"msg", "could not generate macOS package for tenant",
					"tenant", tenant,
					"err", err,
				)
				os.Exit(1)
			}
			level.Debug(logger).Log(
				"msg", "copied macOS package for tenant and hostname",
				"path", destinationPath,
				"tenant", tenant,
				"hostname", hostname,
			)
		}
	}

	logger.Log(
		"msg", "package generation complete",
		"path", uploadRoot,
	)

	if err := packaging.GsutilRsync(uploadRoot, "gs://packaging/"); err != nil {
		logger.Log(
			"msg", "could not upload files to GCS",
			"err", err,
		)
		os.Exit(1)
	}

	if err := os.RemoveAll(uploadRoot); err != nil {
		logger.Log(
			"msg", "could not remove the upload root",
			"err", err,
		)
		os.Exit(1)
	}

	logger.Log("msg", "upload complete")
}

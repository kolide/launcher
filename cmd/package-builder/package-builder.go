package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/tools/packaging"
	"github.com/kolide/launcher/tools/packaging/mirror"
	"github.com/pkg/errors"
)

func runVersion(args []string) error {
	version.PrintFull()
	return nil
}

func runEnrollSecret(args []string) error {
	flagset := flag.NewFlagSet("enroll-secret", flag.ExitOnError)
	var (
		flTenant = flagset.String(
			"tenant",
			env.String("TENANT", ""),
			"the tenant name to generate a secret for (example: dababi)",
		)
		flEnrollSecretSigningKeyPath = flagset.String(
			"enroll_secret_signing_key",
			env.String("ENROLL_SECRET_SIGNING_KEY", ""),
			"the path to the PEM key which is used to sign the enrollment secret JWT token",
		)
	)

	flagset.Usage = usageFor(flagset, "package-builder enroll-secret [flags]")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	enrollSecretSigningKeyPath := *flEnrollSecretSigningKeyPath
	if enrollSecretSigningKeyPath == "" {
		enrollSecretSigningKeyPath = filepath.Join(packaging.LauncherSource(), "/tools/packaging/example_rsa.pem")
	}

	pemKey, err := ioutil.ReadFile(enrollSecretSigningKeyPath)
	if err != nil {
		return errors.Wrap(err, "could not read the supplied key file")
	}

	token, err := packaging.EnrollSecret(*flTenant, pemKey)
	if err != nil {
		return errors.Wrap(err, "could not generate secret")
	}

	fmt.Println(token)

	return nil
}

func runMake(args []string) error {
	flagset := flag.NewFlagSet("macos", flag.ExitOnError)
	var (
		flDebug = flagset.Bool(
			"debug",
			false,
			"enable debug logging",
		)
		flHostname = flagset.String(
			"hostname",
			env.String("HOSTNAME", ""),
			"the hostname of the gRPC server",
		)
		flOsqueryVersion = flagset.String(
			"osquery_version",
			env.String("OSQUERY_VERSION", ""),
			"the osquery version to include in the resultant packages",
		)
		flEnrollSecret = flagset.String(
			"enroll_secret",
			env.String("ENROLL_SECRET", ""),
			"the string to be used as the server enrollment secret",
		)
		flMacPackageSigningKey = flagset.String(
			"mac_package_signing_key",
			env.String("MAC_PACKAGE_SIGNING_KEY", ""),
			"the name of the key that should be used to sign mac packages",
		)
		flInsecure = flagset.Bool(
			"insecure",
			env.Bool("INSECURE", false),
			"whether or not the launcher packages should invoke the launcher's --insecure flag",
		)
		flInsecureGrpc = flagset.Bool(
			"insecure_grpc",
			env.Bool("INSECURE_GRPC", false),
			"whether or not the launcher packages should invoke the launcher's --insecure_grpc flag",
		)
	)

	flagset.Usage = usageFor(flagset, "package-builder make [flags]")
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

	if *flHostname == "" {
		return errors.New("Hostname undefined")
	}

	osqueryVersion := *flOsqueryVersion
	if osqueryVersion == "" {
		osqueryVersion = "stable"
	}

	// TODO check that the signing key is installed if defined
	macPackageSigningKey := *flMacPackageSigningKey
	_ = macPackageSigningKey

	paths, err := packaging.CreatePackages(osqueryVersion, *flHostname, *flEnrollSecret, macPackageSigningKey, *flInsecure, *flInsecureGrpc)
	if err != nil {
		return errors.Wrap(err, "could not generate packages")
	}
	level.Info(logger).Log(
		"msg", "created packages",
		"deb", paths.Deb,
		"rpm", paths.RPM,
		"mac", paths.MacOS,
	)

	return nil
}

func runMirror(args []string) error {
	flagset := flag.NewFlagSet("mirror", flag.ExitOnError)
	flags := mirror.Flags{
		Download: flagset.Bool(
			"download",
			false,
			"Download a fresh copy of Osquery from s3.",
		),
		Platform: flagset.String(
			"platform",
			mirror.PlatformDarwin,
			fmt.Sprintf(
				"Platform to build. Valid values are %s, %s and %s.",
				mirror.PlatformDarwin,
				mirror.PlatformLinux,
				mirror.PlatformWindows,
			),
		),
		Channel: flagset.String(
			"channel",
			mirror.ChannelStable,
			fmt.Sprintf(
				"Create a tarball for a specific autoupdate channel. Valid values: %s, %s and %s.",
				mirror.ChannelBeta,
				mirror.ChannelStable,
				mirror.ChannelNightly,
			),
		),
		Extract: flagset.Bool(
			"extract",
			false,
			"Extract Osquery binary from package.",
		),
		OsqueryTarball: flagset.Bool(
			"osquery-tarball",
			false,
			"Create a tarball from Osquery binary.",
		),
		OsqueryMirrorUpload: flagset.Bool(
			"osquery-upload",
			false,
			"Upload Osquery tarball to mirror.",
		),
		OsqueryNotaryPublish: flagset.Bool(
			"osquery-publish",
			false,
			"Publish Osquery target to Notary.",
		),
		LauncherTarball: flagset.Bool(
			"launcher-tarball",
			false,
			"Create a tarball from Launcher build.",
		),
		LauncherUpload: flagset.Bool(
			"launcher-upload",
			false,
			"Upload Launcher tarball to mirror.",
		),
		LauncherPublish: flagset.Bool(
			"launcher-publish",
			false,
			"Publish Launcher tarball to Notary.",
		),
	}

	var (
		flDebug = flagset.Bool(
			"debug",
			false,
			"Enable debug logging.",
		)
		flAll = flagset.Bool(
			"all",
			true,
			"Complete build and publish of Osquery and Launcher. If false, operations are enabled individually.",
		)
	)

	flagset.Usage = usageFor(flagset, "package-builder mirror [flags]")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.Caller(5))

	if *flDebug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	// If 'all' is set, all operations are enabled, platform, debug, and
	// channel may still be set to non-default values.
	if *flAll {
		flags = mirror.ToggleAllOperations(flags)
	}

	if err := mirror.Publish(logger, flags); err != nil {
		return err
	}

	return nil
}

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
			env.String("OSQUERY_VERSION", ""),
			"the osquery version to include in the resultant packages",
		)
		flEnrollSecretSigningKeyPath = flagset.String(
			"enroll_secret_signing_key",
			env.String("enroll_secret_signing_key", ""),
			"the path to the PEM key which is used to sign the enrollment secret JWT token",
		)
		flMacPackageSigningKey = flagset.String(
			"mac_package_signing_key",
			env.String("MAC_PACKAGE_SIGNING_KEY", ""),
			"the name of the key that should be used to sign mac packages",
		)
		flPRStart = flagset.Int(
			"pr_start",
			0,
			"the PR to start building the dev packages at",
		)
		flPREnd = flagset.Int(
			"pr_end",
			0,
			"the PR to build the dev packages to",
		)
		flNumberOfTenants = flagset.Int(
			"number_of_tenants",
			3,
			"the number of tenants to generate dev packages for",
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

	if *flPRStart == 0 || *flPREnd == 0 {
		return errors.New("pr_start and pr_end flags must be defined to build dev packages")
	}

	osqueryVersion := *flOsqueryVersion
	if osqueryVersion == "" {
		osqueryVersion = "stable"
	}

	enrollSecretSigningKeyPath := *flEnrollSecretSigningKeyPath
	if enrollSecretSigningKeyPath == "" {
		enrollSecretSigningKeyPath = filepath.Join(packaging.LauncherSource(), "/tools/packaging/example_rsa.pem")
	}

	if _, err := os.Stat(enrollSecretSigningKeyPath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrap(err, "key file doesn't exist")
		} else {
			return errors.Wrap(err, "could not stat key file")
		}
	}
	pemKey, err := ioutil.ReadFile(enrollSecretSigningKeyPath)
	if err != nil {
		return errors.Wrap(err, "could not read the supplied key file")
	}

	// TODO check that the signing key is installed if defined
	macPackageSigningKey := *flMacPackageSigningKey
	_ = macPackageSigningKey

	// Generate packages for PRs
	prToStartFrom, prToGenerateUntil := *flPRStart, *flPREnd
	firstID, numberOfIDsToGenerate := 100001, *flNumberOfTenants

	level.Debug(logger).Log(
		"msg", "creating packages for PRs",
		"pr_to_start_from", prToStartFrom,
		"pr_to_generate_until", prToGenerateUntil,
		"number_of_tenants_to_generate", numberOfIDsToGenerate,
	)

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
			paths, err := packaging.CreateKolidePackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey, macPackageSigningKey)
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
			paths, err := packaging.CreateKolidePackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey, macPackageSigningKey)
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

	if err := packaging.SetGCPProject("kolide-ose-testing"); err != nil {
		return errors.Wrap(err, "could not set GCP project")
	}

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
			env.String("OSQUERY_VERSION", ""),
			"the osquery version to include in the resultant packages",
		)
		flEnrollSecretSigningKeyPath = flagset.String(
			"enroll_secret_signing_key",
			env.String("enroll_secret_signing_key", ""),
			"the path to the PEM key which is used to sign the enrollment secret JWT token",
		)
		flMacPackageSigningKey = flagset.String(
			"mac_package_signing_key",
			env.String("MAC_PACKAGE_SIGNING_KEY", ""),
			"the name of the key that should be used to sign mac packages",
		)
		flNumberOfTenants = flagset.Int(
			"number_of_tenants",
			3,
			"the number of tenants to generate dev packages for",
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

	enrollSecretSigningKeyPath := *flEnrollSecretSigningKeyPath
	if enrollSecretSigningKeyPath == "" {
		enrollSecretSigningKeyPath = filepath.Join(packaging.LauncherSource(), "/tools/packaging/example_rsa.pem")
	}

	if _, err := os.Stat(enrollSecretSigningKeyPath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrap(err, "key file doesn't exist")
		} else {
			return errors.Wrap(err, "could not stat key file")
		}
	}
	pemKey, err := ioutil.ReadFile(enrollSecretSigningKeyPath)
	if err != nil {
		return errors.Wrap(err, "could not read the supplied key file")
	}

	// TODO check that the signing key is installed if defined
	macPackageSigningKey := *flMacPackageSigningKey
	_ = macPackageSigningKey

	firstID, numberOfIDsToGenerate := 100001, *flNumberOfTenants

	additionalExtras := []int{
		100155,
		100164,
		100174,
		100184,
	}

	tenantIDs := []int{}
	for id := firstID; id < firstID+numberOfIDsToGenerate; id++ {
		tenantIDs = append(tenantIDs, id)
	}
	for _, id := range additionalExtras {
		tenantIDs = append(tenantIDs, id)
	}

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
		for _, id := range tenantIDs {
			tenant := packaging.TenantName(id)
			paths, err := packaging.CreateKolidePackages(uploadRoot, osqueryVersion, hostname, tenant, pemKey, macPackageSigningKey)
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

	if err := packaging.SetGCPProject("kolide-website"); err != nil {
		return errors.Wrap(err, "could not set GCP project")
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

func usageFor(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "USAGE\n")
		fmt.Fprintf(os.Stderr, "  %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "\t-%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "USAGE\n")
	fmt.Fprintf(os.Stderr, "  %s <mode> --help\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "MODES\n")
	fmt.Fprintf(os.Stderr, "  make         Generate a single launcher package for each platform\n")
	fmt.Fprintf(os.Stderr, "  dev          Generate development launcher packages and upload them to GCS\n")
	fmt.Fprintf(os.Stderr, "  prod         Generate production launcher packages and upload them to GCS\n")
	fmt.Fprintf(os.Stderr, "  version      Print full version information\n")
	fmt.Fprintf(os.Stderr, "  mirror       Manage the local mirror for binaries.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "VERSION\n")
	fmt.Fprintf(os.Stderr, "  %s\n", version.Version().Version)
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var run func([]string) error
	switch strings.ToLower(os.Args[1]) {
	case "version":
		run = runVersion
	case "mirror":
		run = runMirror
	case "make":
		run = runMake
	case "dev":
		run = runDev
	case "prod":
		run = runProd
	case "enroll-secret":
		run = runEnrollSecret
	default:
		usage()
		os.Exit(1)
	}

	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

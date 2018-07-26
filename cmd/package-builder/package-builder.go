package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/pkg/errors"
)

func runVersion(args []string) error {
	version.PrintFull()
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
		flAutoupdate = flagset.Bool(
			"autoupdate",
			env.Bool("AUTOUPDATE", false),
			"whether or not the launcher packages should invoke the launcher's --autoupdate flag",
		)
		flUpdateChannel = flagset.String(
			"update_channel",
			env.String("UPDATE_CHANNEL", ""),
			"the value that should be used when invoking the launcher's --update_channel flag",
		)
		flControl = flagset.Bool(
			"control",
			env.Bool("CONTROL", false),
			"whether or not the launcher packages should invoke the launcher's --control flag",
		)
		flControlHostname = flagset.String(
			"control_hostname",
			env.String("CONTROL_HOSTNAME", ""),
			"the value that should be used when invoking the launcher's --control_hostname flag",
		)
		flDisableControlTLS = flagset.Bool(
			"disable_control_tls",
			env.Bool("DISABLE_CONTROL_TLS", false),
			"whether or not the launcher packages should invoke the launcher's --disable_control_tls flag",
		)
		flIdentifier = flagset.String(
			"identifier",
			env.String("IDENTIFIER", "launcher"),
			"the name of the directory that the launcher installation will shard into",
		)
		flOmitSecret = flagset.Bool(
			"omit_secret",
			env.Bool("OMIT_SECRET", false),
			"omit the enroll secret in the resultant package (default: false)",
		)
		flSystemd = flagset.Bool(
			"systemd",
			env.Bool("SYSTEMD", true),
			"weather or not the launcher packages should be built to target systems with systemd (default: true)",
		)
		flCertPins = flagset.String(
			"cert_pins",
			env.String("CERT_PINS", ""),
			"Comma separated, hex encoded SHA256 hashes of pinned subject public key info",
		)
		flRootPEM = flagset.String(
			"root_pem",
			env.String("ROOT_PEM", ""),
			"Path to PEM file including root certificates to verify against",
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

	// Validate that pinned certs are valid hex
	for _, pin := range strings.Split(*flCertPins, ",") {
		if _, err := hex.DecodeString(pin); err != nil {
			return errors.Wrap(err, "unable to parse cert pins")
		}
	}

	currentVersion := version.Version().Version
	paths, err := packaging.CreatePackages(
		currentVersion,
		osqueryVersion,
		*flHostname,
		*flEnrollSecret,
		macPackageSigningKey,
		*flInsecure,
		*flInsecureGrpc,
		*flAutoupdate,
		*flUpdateChannel,
		*flControl,
		*flControlHostname,
		*flDisableControlTLS,
		*flIdentifier,
		*flOmitSecret,
		*flSystemd,
		*flCertPins,
		*flRootPEM,
	)
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
	fmt.Fprintf(os.Stderr, "  version      Print full version information\n")
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
	case "make":
		run = runMake
	default:
		usage()
		os.Exit(1)
	}

	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/autoupdate"
	"github.com/pkg/errors"
)

// options is the set of configurable options that may be set when launching this
// program
type options struct {
	kolideServerURL    string
	enrollSecret       string
	enrollSecretPath   string
	rootDirectory      string
	osquerydPath       string
	certPins           [][]byte
	rootPEM            string
	loggingInterval    time.Duration
	autoupdate         bool
	printVersion       bool
	developerUsage     bool
	debug              bool
	insecureTLS        bool
	insecureGRPC       bool
	notaryServerURL    string
	mirrorServerURL    string
	autoupdateInterval time.Duration
	updateChannel      autoupdate.UpdateChannel
}

const (
	defaultRootDirectory = "launcher-root"
)

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
		// Primary options
		flRootDirectory = flag.String(
			"root_directory",
			env.String("KOLIDE_LAUNCHER_ROOT_DIRECTORY", ""),
			"The location of the local database, pidfiles, etc.",
		)
		flKolideServerURL = flag.String(
			"hostname",
			env.String("KOLIDE_LAUNCHER_HOSTNAME", ""),
			"The hostname of the gRPC server",
		)
		flEnrollSecret = flag.String(
			"enroll_secret",
			env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", ""),
			"The enroll secret that is used in your environment",
		)
		flEnrollSecretPath = flag.String(
			"enroll_secret_path",
			env.String("KOLIDE_LAUNCHER_ENROLL_SECRET_PATH", ""),
			"Optionally, the path to your enrollment secret",
		)
		flOsquerydPath = flag.String(
			"osqueryd_path",
			env.String("KOLIDE_LAUNCHER_OSQUERYD_PATH", ""),
			"Path to the osqueryd binary to use (Default: find osqueryd in $PATH)",
		)
		flCertPins = flag.String(
			"cert_pins",
			env.String("KOLIDE_LAUNCHER_CERT_PINS", ""),
			"Comma separated, hex encoded SHA256 hashes of pinned subject public key info",
		)
		flRootPEM = flag.String(
			"root_pem",
			env.String("KOLIDE_LAUNCHER_ROOT_PEM", ""),
			"Path to PEM file including root certificates to verify against",
		)
		flLoggingInterval = flag.Duration(
			"logging_interval",
			env.Duration("KOLIDE_LAUNCHER_LOGGING_INTERVAL", 60*time.Second),
			"The interval at which logs should be flushed to the server",
		)

		// Autoupdate options
		flAutoupdate = flag.Bool(
			"autoupdate",
			env.Bool("KOLIDE_LAUNCHER_AUTOUPDATE", false),
			"Whether or not the osquery autoupdater is enabled (default: false)",
		)
		flNotaryServerURL = flag.String(
			"notary_url",
			env.String("KOLIDE_LAUNCHER_NOTARY_SERVER_URL", autoupdate.DefaultNotary),
			"The Notary update server (default: https://notary.kolide.co)",
		)
		flMirrorURL = flag.String(
			"mirror_url",
			env.String("KOLIDE_LAUNCHER_MIRROR_SERVER_URL", autoupdate.DefaultMirror),
			"The mirror server for autoupdates (default: https://dl.kolide.co)",
		)
		flAutoupdateInterval = flag.Duration(
			"autoupdate_interval",
			duration("KOLIDE_LAUNCHER_AUTOUPDATE_INTERVAL", 1*time.Hour),
			"The interval to check for updates (default: once every hour)",
		)
		flUpdateChannel = flag.String(
			"update_channel",
			env.String("KOLIDE_LAUNCHER_UPDATE_CHANNEL", "stable"),
			"The channel to pull updates from (options: stable, beta, nightly)",
		)

		// Development options
		flDebug = flag.Bool(
			"debug",
			env.Bool("KOLIDE_LAUNCHER_DEBUG", false),
			"Whether or not debug logging is enabled (default: false)",
		)
		flInsecureTLS = flag.Bool(
			"insecure",
			env.Bool("KOLIDE_LAUNCHER_INSECURE", false),
			"Do not verify TLS certs for outgoing connections (default: false)",
		)
		flInsecureGRPC = flag.Bool(
			"insecure_grpc",
			env.Bool("KOLIDE_LAUNCHER_INSECURE_GRPC", false),
			"Dial GRPC without a TLS config (default: false)",
		)

		// Version command: launcher --version
		flVersion = flag.Bool(
			"version",
			env.Bool("KOLIDE_LAUNCHER_VERSION", false),
			"Print Launcher version and exit",
		)

		// Developer usage
		flDeveloperUsage = flag.Bool(
			"dev_help",
			env.Bool("KOLIDE_LAUNCHER_DEV_HELP", false),
			"Print full Launcher help, including developer options",
		)
	)

	flag.Usage = usage

	flag.Parse()

	// if an osqueryd path was not set, it's likely that we want to use the bundled
	// osqueryd path, but if it cannot be found, we will fail back to using an
	// osqueryd found in the path
	osquerydPath := *flOsquerydPath
	if *flOsquerydPath == "" {
		if _, err := os.Stat(defaultOsquerydPath); err == nil {
			osquerydPath = defaultOsquerydPath
		} else if path, err := exec.LookPath("osqueryd"); err == nil {
			osquerydPath = path
		} else {
			return nil, errors.New("Could not find osqueryd binary")
		}
	}

	if *flEnrollSecret != "" && *flEnrollSecretPath != "" {
		return nil, errors.New("Both enroll_secret and enroll_secret_path were defined")
	}

	updateChannel := autoupdate.Stable
	switch *flUpdateChannel {
	case "stable":
		updateChannel = autoupdate.Stable
	case "beta":
		updateChannel = autoupdate.Beta
	case "nightly":
		updateChannel = autoupdate.Nightly
	default:
		return nil, fmt.Errorf("unknown update channel %s", *flUpdateChannel)
	}

	certPins, err := parseCertPins(*flCertPins)
	if err != nil {
		return nil, err
	}

	opts := &options{
		kolideServerURL:    *flKolideServerURL,
		enrollSecret:       *flEnrollSecret,
		enrollSecretPath:   *flEnrollSecretPath,
		rootDirectory:      *flRootDirectory,
		osquerydPath:       osquerydPath,
		certPins:           certPins,
		rootPEM:            *flRootPEM,
		loggingInterval:    *flLoggingInterval,
		autoupdate:         *flAutoupdate,
		printVersion:       *flVersion,
		developerUsage:     *flDeveloperUsage,
		debug:              *flDebug,
		insecureTLS:        *flInsecureTLS,
		insecureGRPC:       *flInsecureGRPC,
		notaryServerURL:    *flNotaryServerURL,
		mirrorServerURL:    *flMirrorURL,
		autoupdateInterval: *flAutoupdateInterval,
		updateChannel:      updateChannel,
	}
	return opts, nil
}

func shortUsage() {
	launcherFlags := map[string]string{}
	flagAggregator := func(f *flag.Flag) {
		launcherFlags[f.Name] = f.Usage
	}
	flag.VisitAll(flagAggregator)

	printOpt := func(opt string) {
		fmt.Fprintf(os.Stderr, "  --%s", opt)
		for i := 0; i < 22-len(opt); i++ {
			fmt.Fprintf(os.Stderr, " ")
		}
		fmt.Fprintf(os.Stderr, "%s\n", launcherFlags[opt])
	}

	fmt.Fprintf(os.Stderr, "The Osquery Launcher, by Kolide (version %s)\n", version.Version().Version)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Usage: launcher --option=value\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("hostname")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("enroll_secret")
	printOpt("enroll_secret_path")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("root_directory")
	printOpt("osqueryd_path")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("autoupdate")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("version")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  All options can be set as environment variables using the following convention:\n")
	fmt.Fprintf(os.Stderr, "      KOLIDE_LAUNCHER_OPTION=value launcher\n")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("dev_help")
	fmt.Fprintf(os.Stderr, "\n")
}

func usage() {
	shortUsage()
	usageFooter()
}

func developerUsage() {
	launcherFlags := map[string]string{}
	flagAggregator := func(f *flag.Flag) {
		launcherFlags[f.Name] = f.Usage
	}
	flag.VisitAll(flagAggregator)

	printOpt := func(opt string) {
		fmt.Fprintf(os.Stderr, "  --%s", opt)
		for i := 0; i < 22-len(opt); i++ {
			fmt.Fprintf(os.Stderr, " ")
		}
		fmt.Fprintf(os.Stderr, "%s\n", launcherFlags[opt])
	}

	shortUsage()
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Development Options:\n")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("debug")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("insecure")
	printOpt("insecure_grpc")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("logging_interval")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("notary_url")
	printOpt("mirror_url")
	printOpt("autoupdate_interval")
	printOpt("update_channel")
	fmt.Fprintf(os.Stderr, "\n")
	usageFooter()
}

func usageFooter() {
	fmt.Fprintf(os.Stderr, "For more information, check out https://kolide.co/osquery\n")
	fmt.Fprintf(os.Stderr, "\n")
}

// TODO: move to kolide/kit and figure out error handling there.
func duration(key string, def time.Duration) time.Duration {
	if env, ok := os.LookupEnv(key); ok {
		t, err := time.ParseDuration(env)
		if err != nil {
			fmt.Println("env: parse duration flag: ", err)
			os.Exit(1)
		}
		return t
	}
	return def
}

func parseCertPins(pins string) ([][]byte, error) {
	var certPins [][]byte
	if pins != "" {
		for _, hexPin := range strings.Split(pins, ",") {
			pin, err := hex.DecodeString(hexPin)
			if err != nil {
				return nil, errors.Wrap(err, "decoding cert pin")
			}
			certPins = append(certPins, pin)
		}
	}
	return certPins, nil
}

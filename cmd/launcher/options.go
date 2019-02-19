package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

// options is the set of configurable options that may be set when launching this
// program
type options struct {
	kolideServerURL     string
	enrollSecret        string
	enrollSecretPath    string
	rootDirectory       string
	osquerydPath        string
	certPins            [][]byte
	rootPEM             string
	loggingInterval     time.Duration
	enableInitialRunner bool

	control           bool
	controlServerURL  string
	getShellsInterval time.Duration

	autoupdate         bool
	printVersion       bool
	developerUsage     bool
	debug              bool
	disableControlTLS  bool
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

	flagset := flag.NewFlagSet("launcher", flag.ExitOnError)
	flagset.Usage = usage

	var (
		// Primary options
		flRootDirectory = flagset.String(
			"root_directory",
			"",
			"The location of the local database, pidfiles, etc.",
		)
		flKolideServerURL = flagset.String(
			"hostname",
			"",
			"The hostname of the gRPC server",
		)

		flControl = flagset.Bool(
			"control",
			false,
			"Whether or not the control server is enabled (default: false)",
		)
		flControlServerURL = flagset.String(
			"control_hostname",
			"",
			"The hostname of the control server",
		)
		flGetShellsInterval = flagset.Duration(
			"control_get_shells_interval",
			3*time.Second,
			"The interval at which the get shells request will be made",
		)

		flEnrollSecret = flagset.String(
			"enroll_secret",
			"",
			"The enroll secret that is used in your environment",
		)
		flEnrollSecretPath = flagset.String(
			"enroll_secret_path",
			"",
			"Optionally, the path to your enrollment secret",
		)
		flOsquerydPath = flagset.String(
			"osqueryd_path",
			"",
			"Path to the osqueryd binary to use (Default: find osqueryd in $PATH)",
		)
		flCertPins = flagset.String(
			"cert_pins",
			"",
			"Comma separated, hex encoded SHA256 hashes of pinned subject public key info",
		)
		flRootPEM = flagset.String(
			"root_pem",
			"",
			"Path to PEM file including root certificates to verify against",
		)
		flLoggingInterval = flagset.Duration(
			"logging_interval",
			60*time.Second,
			"The interval at which logs should be flushed to the server",
		)

		// Autoupdate options
		flAutoupdate = flagset.Bool(
			"autoupdate",
			false,
			"Whether or not the osquery autoupdater is enabled (default: false)",
		)
		flNotaryServerURL = flagset.String(
			"notary_url",
			autoupdate.DefaultNotary,
			"The Notary update server (default: https://notary.kolide.co)",
		)
		flMirrorURL = flagset.String(
			"mirror_url",
			autoupdate.DefaultMirror,
			"The mirror server for autoupdates (default: https://dl.kolide.co)",
		)
		flAutoupdateInterval = flagset.Duration(
			"autoupdate_interval",
			1*time.Hour,
			"The interval to check for updates (default: once every hour)",
		)
		flUpdateChannel = flagset.String(
			"update_channel",
			"stable",
			"The channel to pull updates from (options: stable, beta, nightly)",
		)

		// Development options
		flDebug = flagset.Bool(
			"debug",
			false,
			"Whether or not debug logging is enabled (default: false)",
		)
		flDisableControlTLS = flagset.Bool(
			"disable_control_tls",
			false,
			"Disable TLS encryption for the control features",
		)
		flInsecureTLS = flagset.Bool(
			"insecure",
			false,
			"Do not verify TLS certs for outgoing connections (default: false)",
		)
		flInsecureGRPC = flagset.Bool(
			"insecure_grpc",
			false,
			"Dial GRPC without a TLS config (default: false)",
		)

		// Version command: launcher --version
		flVersion = flagset.Bool(
			"version",
			false,
			"Print Launcher version and exit",
		)

		// Developer usage
		flDeveloperUsage = flagset.Bool(
			"dev_help",
			false,
			"Print full Launcher help, including developer options",
		)

		// Enable Initial Runner: launcher --with_initial_runner
		flInitialRunner = flagset.Bool(
			"with_initial_runner",
			false,
			"Run differential queries from config ahead of scheduled interval.",
		)
	)

	ff.Parse(flagset, os.Args[1:],
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
		ff.WithEnvVarPrefix("KOLIDE_LAUNCHER"),
	)

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
	case "", "stable":
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
		kolideServerURL:     *flKolideServerURL,
		control:             *flControl,
		controlServerURL:    *flControlServerURL,
		getShellsInterval:   *flGetShellsInterval,
		enrollSecret:        *flEnrollSecret,
		enrollSecretPath:    *flEnrollSecretPath,
		rootDirectory:       *flRootDirectory,
		osquerydPath:        osquerydPath,
		certPins:            certPins,
		rootPEM:             *flRootPEM,
		loggingInterval:     *flLoggingInterval,
		enableInitialRunner: *flInitialRunner,
		autoupdate:          *flAutoupdate,
		printVersion:        *flVersion,
		developerUsage:      *flDeveloperUsage,
		debug:               *flDebug,
		disableControlTLS:   *flDisableControlTLS,
		insecureTLS:         *flInsecureTLS,
		insecureGRPC:        *flInsecureGRPC,
		notaryServerURL:     *flNotaryServerURL,
		mirrorServerURL:     *flMirrorURL,
		autoupdateInterval:  *flAutoupdateInterval,
		updateChannel:       updateChannel,
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
	printOpt("control")
	printOpt("control_hostname")
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
	printOpt("control_get_shells_interval")
	printOpt("disable_control_tls")
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

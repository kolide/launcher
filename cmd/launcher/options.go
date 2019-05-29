package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

const (
	defaultRootDirectory = "launcher-root"
	skipEnvParse         = runtime.GOOS == "windows" // skip environmental variable parsing on windows
)

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions(args []string) (*launcher.Options, error) {
	flagset := flag.NewFlagSet("launcher", flag.ExitOnError)
	flagset.Usage = func() { usage(flagset) }

	var (
		// Primary options
		flCertPins          = flagset.String("cert_pins", "", "Comma separated, hex encoded SHA256 hashes of pinned subject public key info")
		flControl           = flagset.Bool("control", false, "Whether or not the control server is enabled (default: false)")
		flControlServerURL  = flagset.String("control_hostname", "", "The hostname of the control server")
		flEnrollSecret      = flagset.String("enroll_secret", "", "The enroll secret that is used in your environment")
		flEnrollSecretPath  = flagset.String("enroll_secret_path", "", "Optionally, the path to your enrollment secret")
		flGetShellsInterval = flagset.Duration("control_get_shells_interval", 3*time.Second, "The interval at which the 'get shells' request will be made")
		flInitialRunner     = flagset.Bool("with_initial_runner", false, "Run differential queries from config ahead of scheduled interval.")
		flKolideServerURL   = flagset.String("hostname", "", "The hostname of the gRPC server")
		flTransport         = flagset.String("transport", "grpc", "The transport protocol that should be used to communicate with remote (default: grpc)")
		flLoggingInterval   = flagset.Duration("logging_interval", 60*time.Second, "The interval at which logs should be flushed to the server")
		flOsquerydPath      = flagset.String("osqueryd_path", "", "Path to the osqueryd binary to use (Default: find osqueryd in $PATH)")
		flRootDirectory     = flagset.String("root_directory", "", "The location of the local database, pidfiles, etc.")
		flRootPEM           = flagset.String("root_pem", "", "Path to PEM file including root certificates to verify against")
		flVersion           = flagset.Bool("version", false, "Print Launcher version and exit")
		_                   = flagset.String("config", "", "config file to parse options from (optional)")

		// Autoupdate options
		flAutoupdate         = flagset.Bool("autoupdate", false, "Whether or not the osquery autoupdater is enabled (default: false)")
		flNotaryServerURL    = flagset.String("notary_url", autoupdate.DefaultNotary, "The Notary update server (default: https://notary.kolide.co)")
		flMirrorURL          = flagset.String("mirror_url", autoupdate.DefaultMirror, "The mirror server for autoupdates (default: https://dl.kolide.co)")
		flAutoupdateInterval = flagset.Duration("autoupdate_interval", 1*time.Hour, "The interval to check for updates (default: once every hour)")
		flUpdateChannel      = flagset.String("update_channel", "stable", "The channel to pull updates from (options: stable, beta, nightly)")
		flNotaryPrefix       = flagset.String("notary_prefix", autoupdate.DefaultNotaryPrefix, "The prefix for Notary path that contains the collections (default: kolide/)")

		// Development options
		flDebug             = flagset.Bool("debug", false, "Whether or not debug logging is enabled (default: false)")
		flDeveloperUsage    = flagset.Bool("dev_help", false, "Print full Launcher help, including developer options")
		flDisableControlTLS = flagset.Bool("disable_control_tls", false, "Disable TLS encryption for the control features")
		flInsecureTransport = flagset.Bool("insecure_transport", false, "Do not use TLS for transport layer (default: false)")
		flInsecureTLS       = flagset.Bool("insecure", false, "Do not verify TLS certs for outgoing connections (default: false)")
	)

	ffOpts := []ff.Option{
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
	}

	// Windows doesn't really support environmental variables in quite
	// the same way unix does. This led to Kolide's early Cloud packages
	// installing with some global environmental variables. Those would
	// cause an incompatibility with all subsequent launchers. As
	// they're not part of the normal windows use case, we can skip
	// using them here.
	if !skipEnvParse {
		ffOpts = append(ffOpts, ff.WithEnvVarPrefix("KOLIDE_LAUNCHER"))
	}

	ff.Parse(flagset, args, ffOpts...)

	// handle --version
	if *flVersion {
		version.PrintFull()
		os.Exit(0)
	}

	// handle --usage
	if *flDeveloperUsage {
		developerUsage(flagset)
		os.Exit(0)
	}

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

	opts := &launcher.Options{
		KolideServerURL:     *flKolideServerURL,
		Transport:           *flTransport,
		Control:             *flControl,
		ControlServerURL:    *flControlServerURL,
		GetShellsInterval:   *flGetShellsInterval,
		EnrollSecret:        *flEnrollSecret,
		EnrollSecretPath:    *flEnrollSecretPath,
		RootDirectory:       *flRootDirectory,
		OsquerydPath:        osquerydPath,
		CertPins:            certPins,
		RootPEM:             *flRootPEM,
		LoggingInterval:     *flLoggingInterval,
		EnableInitialRunner: *flInitialRunner,
		Autoupdate:          *flAutoupdate,
		Debug:               *flDebug,
		DisableControlTLS:   *flDisableControlTLS,
		InsecureTLS:         *flInsecureTLS,
		InsecureTransport:   *flInsecureTransport,
		NotaryServerURL:     *flNotaryServerURL,
		MirrorServerURL:     *flMirrorURL,
		NotaryPrefix:        *flNotaryPrefix,
		AutoupdateInterval:  *flAutoupdateInterval,
		UpdateChannel:       updateChannel,
	}
	return opts, nil
}

func shortUsage(flagset *flag.FlagSet) {
	launcherFlags := map[string]string{}
	flagAggregator := func(f *flag.Flag) {
		launcherFlags[f.Name] = f.Usage
	}
	flagset.VisitAll(flagAggregator)

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
	printOpt("transport")
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
	if !skipEnvParse {
		fmt.Fprintf(os.Stderr, "  All options can be set as environment variables using the following convention:\n")
		fmt.Fprintf(os.Stderr, "      KOLIDE_LAUNCHER_OPTION=value launcher\n")
		fmt.Fprintf(os.Stderr, "\n")
	}
	printOpt("dev_help")
	fmt.Fprintf(os.Stderr, "\n")
}

func usage(flagset *flag.FlagSet) {
	shortUsage(flagset)
	usageFooter()
}

func developerUsage(flagset *flag.FlagSet) {
	launcherFlags := map[string]string{}
	flagAggregator := func(f *flag.Flag) {
		launcherFlags[f.Name] = f.Usage
	}
	flagset.VisitAll(flagAggregator)

	printOpt := func(opt string) {
		fmt.Fprintf(os.Stderr, "  --%s", opt)
		for i := 0; i < 22-len(opt); i++ {
			fmt.Fprintf(os.Stderr, " ")
		}
		fmt.Fprintf(os.Stderr, "%s\n", launcherFlags[opt])
	}

	shortUsage(flagset)

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Development Options:\n")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("debug")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("insecure")
	printOpt("insecure_transport")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("logging_interval")
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("notary_url")
	printOpt("mirror_url")
	printOpt("autoupdate_interval")
	printOpt("update_channel")
	printOpt("notary_prefix")
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

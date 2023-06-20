package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/peterbourgon/ff/v3"
)

const (
	defaultRootDirectory = "launcher-root"
	skipEnvParse         = runtime.GOOS == "windows" // skip environmental variable parsing on windows
)

var (
	// When launcher proper runs, it's expected that these defaults are their zero values
	// However, special launcher subcommands such as launcher doctor can override these
	defaultRootDirectoryPath string
	defaultEtcDirectoryPath  string
	defaultBinDirectoryPath  string
	defaultConfigFilePath    string
	defaultAutoupdate        bool
)

// Adapted from
// https://stackoverflow.com/questions/28322997/how-to-get-a-list-of-values-into-a-flag-in-golang/28323276#28323276
type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, " ")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions(subcommandName string, args []string) (*launcher.Options, error) {
	flagsetName := "launcher"
	if subcommandName != "" {
		flagsetName = fmt.Sprintf("launcher %s", subcommandName)
	}
	flagset := flag.NewFlagSet(flagsetName, flag.ExitOnError)
	if subcommandName != "" {
		flagset.Usage = func() { usage(flagset) }
	} else {
		flagset.Usage = commandUsage(flagset, flagsetName)
	}

	var (
		// Primary options
		flAutoloadedExtensions   arrayFlags
		flCertPins               = flagset.String("cert_pins", "", "Comma separated, hex encoded SHA256 hashes of pinned subject public key info")
		flControlRequestInterval = flagset.Duration("control_request_interval", 60*time.Second, "The interval at which the control server requests will be made")
		flEnrollSecret           = flagset.String("enroll_secret", "", "The enroll secret that is used in your environment")
		flEnrollSecretPath       = flagset.String("enroll_secret_path", "", "Optionally, the path to your enrollment secret")
		flInitialRunner          = flagset.Bool("with_initial_runner", false, "Run differential queries from config ahead of scheduled interval.")
		flKolideServerURL        = flagset.String("hostname", "", "The hostname of the gRPC server")
		flKolideHosted           = flagset.Bool("kolide_hosted", false, "Use Kolide SaaS settings for defaults")
		flTransport              = flagset.String("transport", "grpc", "The transport protocol that should be used to communicate with remote (default: grpc)")
		flLoggingInterval        = flagset.Duration("logging_interval", 60*time.Second, "The interval at which logs should be flushed to the server")
		flOsquerydPath           = flagset.String("osqueryd_path", "", "Path to the osqueryd binary to use (Default: find osqueryd in $PATH)")
		flRootDirectory          = flagset.String("root_directory", defaultRootDirectoryPath, "The location of the local database, pidfiles, etc.")
		flRootPEM                = flagset.String("root_pem", "", "Path to PEM file including root certificates to verify against")
		flVersion                = flagset.Bool("version", false, "Print Launcher version and exit")
		flLogMaxBytesPerBatch    = flagset.Int("log_max_bytes_per_batch", 0, "Maximum size of a batch of logs. Recommend leaving unset, and launcher will determine")
		flOsqueryFlags           arrayFlags // set below with flagset.Var
		flCompactDbMaxTx         = flagset.Int64("compactdb-max-tx", 65536, "Maximum transaction size used when compacting the internal DB")
		flConfigFilePath         = flagset.String("config", defaultConfigFilePath, "config file to parse options from (optional)")
		flExportTraces           = flagset.Bool("export_traces", false, "Whether to export traces")
		flTraceSamplingRate      = flagset.Float64("trace_sampling_rate", 0.0, "What fraction of traces should be sampled")
		flIngestServerURL        = flagset.String("ingest_url", "", "Where to export traces and logs")
		flDisableIngestTLS       = flagset.Bool("disable_ingest_tls", false, "Disable TLS for observability ingest server communication")

		// osquery TLS endpoints
		flOsqTlsConfig    = flagset.String("config_tls_endpoint", "", "Config endpoint for the osquery tls transport")
		flOsqTlsEnroll    = flagset.String("enroll_tls_endpoint", "", "Enroll endpoint for the osquery tls transport")
		flOsqTlsLogger    = flagset.String("logger_tls_endpoint", "", "Logger endpoint for the osquery tls transport")
		flOsqTlsDistRead  = flagset.String("distributed_tls_read_endpoint", "", "Distributed read endpoint for the osquery tls transport")
		flOsqTlsDistWrite = flagset.String("distributed_tls_write_endpoint", "", "Distributed write endpoint for the osquery tls transport")

		// Autoupdate options
		flAutoupdate             = flagset.Bool("autoupdate", defaultAutoupdate, "Whether or not the osquery autoupdater is enabled (default: false)")
		flNotaryServerURL        = flagset.String("notary_url", autoupdate.DefaultNotary, "The Notary update server (default: https://notary.kolide.co)")
		flTufServerURL           = flagset.String("tuf_url", tuf.DefaultTufServer, "TUF update server (default: https://tuf.kolide.com)")
		flMirrorURL              = flagset.String("mirror_url", autoupdate.DefaultMirror, "The mirror server for autoupdates (default: https://dl.kolide.co)")
		flAutoupdateInterval     = flagset.Duration("autoupdate_interval", 1*time.Hour, "The interval to check for updates (default: once every hour)")
		flUpdateChannel          = flagset.String("update_channel", "stable", "The channel to pull updates from (options: stable, beta, nightly)")
		flNotaryPrefix           = flagset.String("notary_prefix", autoupdate.DefaultNotaryPrefix, "The prefix for Notary path that contains the collections (default: kolide/)")
		flAutoupdateInitialDelay = flagset.Duration("autoupdater_initial_delay", 1*time.Hour, "Initial autoupdater subprocess delay")
		flUpdateDirectory        = flagset.String("update_directory", "", "Local directory to hold updates for osqueryd and launcher")

		// Development & Debugging options
		flDebug                = flagset.Bool("debug", false, "Whether or not debug logging is enabled (default: false)")
		flOsqueryVerbose       = flagset.Bool("osquery_verbose", false, "Enable verbose osqueryd (default: false)")
		flDeveloperUsage       = flagset.Bool("dev_help", false, "Print full Launcher help, including developer options (default: false)")
		flInsecureTransport    = flagset.Bool("insecure_transport", false, "Do not use TLS for transport layer (default: false)")
		flInsecureTLS          = flagset.Bool("insecure", false, "Do not verify TLS certs for outgoing connections (default: false)")
		flIAmBreakingEELicense = flagset.Bool("i-am-breaking-ee-license", false, "Skip license check before running localserver (default: false)")
		flDelayStart           = flagset.Duration("delay_start", 0*time.Second, "How much time to wait before starting launcher")

		// deprecated options, kept for any kind of config file compatibility
		_ = flagset.String("debug_log_file", "", "DEPRECATED")
		_ = flagset.Bool("control", false, "DEPRECATED")
		_ = flagset.String("control_hostname", "", "DEPRECATED")
		_ = flagset.Bool("disable_control_tls", false, "Disable TLS encryption for the control features")
	)

	flagset.Var(&flOsqueryFlags, "osquery_flag", "Flags to pass to osquery (possibly overriding Launcher defaults)")
	flagset.Var(&flAutoloadedExtensions, "autoloaded_extension", "extension paths to autoload, filename without path may be used in same directory as launcher")

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

	// If launcher is using a kolide host, we may override many of
	// the settings. When we're ready, we can _additionally_
	// conditionalize this on the ServerURL to get all the
	// existing deployments
	if *flKolideHosted {
		*flTransport = "osquery"
		*flOsqTlsConfig = "/api/osquery/v0/config"
		*flOsqTlsEnroll = "/api/osquery/v0/enroll"
		*flOsqTlsLogger = "/api/osquery/v0/log"
		*flOsqTlsDistRead = "/api/osquery/v0/distributed/read"
		*flOsqTlsDistWrite = "/api/osquery/v0/distributed/write"
	}

	// if an osqueryd path was not set, it's likely that we want to use the bundled
	// osqueryd path, but if it cannot be found, we will fail back to using an
	// osqueryd found in the path
	osquerydPath := *flOsquerydPath
	if osquerydPath == "" {
		osquerydPath = findOsquery()
		if osquerydPath == "" {
			return nil, errors.New("Could not find osqueryd binary")
		}
	}

	// On windows, we should make sure osquerydPath ends in .exe
	if runtime.GOOS == "windows" && !strings.HasSuffix(osquerydPath, ".exe") {
		osquerydPath = osquerydPath + ".exe"
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
	case "alpha":
		updateChannel = autoupdate.Alpha
	case "nightly":
		updateChannel = autoupdate.Nightly
	default:
		return nil, fmt.Errorf("unknown update channel %s", *flUpdateChannel)
	}

	certPins, err := parseCertPins(*flCertPins)
	if err != nil {
		return nil, err
	}

	// Set control server URL and control server TLS settings based on Kolide server URL, defaulting to local server
	controlServerURL := ""
	insecureControlTLS := false
	disableControlTLS := false

	switch {
	case *flKolideServerURL == "k2device.kolide.com":
		controlServerURL = "k2control.kolide.com"

	case *flKolideServerURL == "k2device-preprod.kolide.com":
		controlServerURL = "k2control-preprod.kolide.com"

	case strings.HasSuffix(*flKolideServerURL, "herokuapp.com"):
		controlServerURL = *flKolideServerURL

	case *flKolideServerURL == "localhost:3443":
		controlServerURL = *flKolideServerURL
		// We don't plumb flRootPEM through to the control server, just disable TLS for now
		insecureControlTLS = true

	case *flKolideServerURL == "localhost:3000" || *flIAmBreakingEELicense:
		controlServerURL = *flKolideServerURL
		disableControlTLS = true
	}

	opts := &launcher.Options{
		Autoupdate:                         *flAutoupdate,
		AutoupdateInterval:                 *flAutoupdateInterval,
		AutoupdateInitialDelay:             *flAutoupdateInitialDelay,
		CertPins:                           certPins,
		CompactDbMaxTx:                     *flCompactDbMaxTx,
		ConfigFilePath:                     *flConfigFilePath,
		Control:                            false,
		ControlServerURL:                   controlServerURL,
		ControlRequestInterval:             *flControlRequestInterval,
		Debug:                              *flDebug,
		DelayStart:                         *flDelayStart,
		DisableControlTLS:                  disableControlTLS,
		InsecureControlTLS:                 insecureControlTLS,
		EnableInitialRunner:                *flInitialRunner,
		EnrollSecret:                       *flEnrollSecret,
		EnrollSecretPath:                   *flEnrollSecretPath,
		ExportTraces:                       *flExportTraces,
		ObservabilityIngestServerURL:       *flIngestServerURL,
		DisableObservabilityIngestTLS:      *flDisableIngestTLS,
		AutoloadedExtensions:               flAutoloadedExtensions,
		IAmBreakingEELicense:               *flIAmBreakingEELicense,
		InsecureTLS:                        *flInsecureTLS,
		InsecureTransport:                  *flInsecureTransport,
		KolideHosted:                       *flKolideHosted,
		KolideServerURL:                    *flKolideServerURL,
		LogMaxBytesPerBatch:                *flLogMaxBytesPerBatch,
		LoggingInterval:                    *flLoggingInterval,
		MirrorServerURL:                    *flMirrorURL,
		NotaryPrefix:                       *flNotaryPrefix,
		NotaryServerURL:                    *flNotaryServerURL,
		TufServerURL:                       *flTufServerURL,
		OsqueryFlags:                       flOsqueryFlags,
		OsqueryTlsConfigEndpoint:           *flOsqTlsConfig,
		OsqueryTlsDistributedReadEndpoint:  *flOsqTlsDistRead,
		OsqueryTlsDistributedWriteEndpoint: *flOsqTlsDistWrite,
		OsqueryTlsEnrollEndpoint:           *flOsqTlsEnroll,
		OsqueryTlsLoggerEndpoint:           *flOsqTlsLogger,
		OsqueryVerbose:                     *flOsqueryVerbose,
		OsquerydPath:                       osquerydPath,
		RootDirectory:                      *flRootDirectory,
		RootPEM:                            *flRootPEM,
		TraceSamplingRate:                  *flTraceSamplingRate,
		Transport:                          *flTransport,
		UpdateChannel:                      updateChannel,
		UpdateDirectory:                    *flUpdateDirectory,
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
		for i := 0; i < 24-len(opt); i++ {
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
	printOpt("log_max_bytes_per_batch")
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
	printOpt("osquery_verbose")
	printOpt("debug_log_file")
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
	fmt.Fprintf(os.Stderr, "\n")
	printOpt("osquery_flag")
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
				return nil, fmt.Errorf("decoding cert pin: %w", err)
			}
			certPins = append(certPins, pin)
		}
	}
	return certPins, nil
}

// findOsquery will attempt to find osquery. We don't much care about
// errors here, either we find it, or we don't.
func findOsquery() string {
	osqBinaryName := "osqueryd"
	if runtime.GOOS == "windows" {
		osqBinaryName = osqBinaryName + ".exe"
	}

	var likelyDirectories []string

	if exPath, err := os.Executable(); err == nil {
		likelyDirectories = append(likelyDirectories, filepath.Dir(exPath))
	}

	// Places to check. We could conditionalize on GOOS, but it doesn't
	// seem important.
	likelyDirectories = append(
		likelyDirectories,
		"/usr/local/kolide/bin",
		"/usr/local/kolide-k2/bin",
		"/usr/local/bin",
		`C:\Program Files\osquery`,
		`C:\Program Files\Kolide\Launcher-kolide-k2\bin`,
	)

	for _, dir := range likelyDirectories {
		maybeOsq := filepath.Join(filepath.Clean(dir), osqBinaryName)

		info, err := os.Stat(maybeOsq)
		if err != nil {
			continue
		}

		if info.IsDir() {
			continue
		}

		// I guess it's good enough...
		return maybeOsq
	}

	// last ditch, check for osquery on the PATH
	if osqPath, err := exec.LookPath(osqBinaryName); err == nil {
		return osqPath
	}

	return ""
}

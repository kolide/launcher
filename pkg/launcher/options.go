package launcher

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
	"text/tabwriter"
	"time"

	"github.com/kolide/kit/version"
	"github.com/peterbourgon/ff/v3"
)

const DefaultLauncherIdentifier string = "kolide-k2"

// Options is the set of options that may be configured for Launcher.
type Options struct {
	// KolideServerURL is the URL of the management server to connect to.
	KolideServerURL string
	// KolideHosted true if using Kolide SaaS settings
	KolideHosted bool
	// EnrollSecret contains the raw enroll secret.
	EnrollSecret string
	// EnrollSecretPath contains the path to a file containing the enroll
	// secret.
	EnrollSecretPath string
	// RootDirectory is the directory that should be used as the osquery
	// root directory (database files, pidfile, etc.).
	RootDirectory string
	// OsquerydPath is the path to the osqueryd binary.
	OsquerydPath string
	// OsqueryHealthcheckStartupDelay is the time to wait before beginning osquery healthchecks
	OsqueryHealthcheckStartupDelay time.Duration
	// CertPins are optional hashes of subject public key info to use for
	// certificate pinning.
	CertPins [][]byte
	// RootPEM is the path to the pem file containing the certificate
	// chain, if necessary for verification.
	RootPEM string
	// LoggingInterval is the interval at which logs should be flushed to
	// the server.
	LoggingInterval time.Duration
	// EnableInitialRunner enables running scheduled queries immediately
	// (before first schedule interval passes).
	EnableInitialRunner bool
	// Transport the transport that should be used for remote
	// communication.
	Transport string
	// LogMaxBytesPerBatch sets the maximum bytes allowed in a batch
	// of log. When blank, launcher will pick a value
	// appropriate for the transport.
	LogMaxBytesPerBatch int

	// Control enables the remote control functionality. It is not in use.
	Control bool
	// ControlServerURL URL for control server.
	ControlServerURL string
	// ControlRequestInterval is the interval at which control client
	// will check for updates from the control server.
	ControlRequestInterval time.Duration

	// Autoupdate enables the autoupdate functionality.
	Autoupdate bool
	// TufServerURL is the URL for the tuf server.
	TufServerURL string
	// MirrorServerURL is the URL for the TUF mirror.
	MirrorServerURL string
	// AutoupdateInterval is the interval at which Launcher will check for
	// updates.
	AutoupdateInterval time.Duration
	// UpdateChannel is the channel to pull options from (stable, beta, nightly).
	UpdateChannel UpdateChannel
	// AutoupdateInitialDelay set an initial startup delay on the autoupdater process.
	AutoupdateInitialDelay time.Duration
	// UpdateDirectory is the location of the update libraries for osqueryd and launcher
	UpdateDirectory string
	// AutoupdateDownloadSplay duration of time over which launcher should select random delay before downloading
	AutoupdateDownloadSplay time.Duration

	// Debug enables debug logging.
	Debug bool
	// Optional file to mirror debug logs to
	DebugLogFile string
	// OsqueryVerbose puts osquery into verbose mode
	OsqueryVerbose bool
	// WatchdogEnabled enables the osquery watchdog
	WatchdogEnabled bool
	// WatchdogDelaySec sets the number of seconds the watchdog will delay on startup before running
	WatchdogDelaySec int
	// WatchdogMemoryLimitMB sets the memory limit on osquery processes
	WatchdogMemoryLimitMB int
	// WatchdogUtilizationLimitPercent sets the CPU utilization limit on osquery processes
	WatchdogUtilizationLimitPercent int

	// OsqueryFlags defines additional flags to pass to osquery (possibly
	// overriding Launcher defaults)
	OsqueryFlags []string
	// DisableControlTLS disables TLS transport with the control server.
	DisableControlTLS bool
	// InsecureControlTLS disables TLS certificate validation for the control server.
	InsecureControlTLS bool
	// InsecureTLS disables TLS certificate verification.
	InsecureTLS bool
	// InsecureTransport disables TLS in the transport layer.
	InsecureTransport bool
	// IAmBreakingEELicence disables the EE licence check before running the local server
	IAmBreakingEELicense bool
	// DelayStart allows for delaying launcher startup for a configurable amount of time
	DelayStart time.Duration
	// ExportTraces enables exporting traces.
	ExportTraces bool
	// TraceSamplingRate is a number between 0.0 and 1.0 that indicates what fraction of traces should be sampled.
	TraceSamplingRate float64
	// LogIngestServerURL is the URL that logs and other observability data will be exported to
	LogIngestServerURL string
	// TraceIngestServerURL is the URL that traces will be exported to
	TraceIngestServerURL string
	// DisableTraceIngestTLS allows for disabling TLS when connecting to the observability ingest server
	DisableTraceIngestTLS bool

	// ConfigFilePath is the config file options were parsed from, if provided
	ConfigFilePath string

	// LocalDevelopmentPath is the path to a local build of launcher to test against, rather than finding the latest version in the library
	LocalDevelopmentPath string

	// Identifier is the key used to identify/namespace a single launcher installation (e.g. kolide-k2)
	Identifier string
}

// ConfigFilePath returns the path to launcher's launcher.flags file. If the path
// is available in the command-line args, it will return that path; otherwise, it
// will fall back to a well-known location.
func ConfigFilePath(args []string) string {
	for i, arg := range args {
		if arg == "--config" || arg == "-config" {
			return strings.Trim(args[i+1], `"'`)
		}

		if strings.Contains(arg, "config=") {
			parts := strings.Split(arg, "=")
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}

	// Not found in command-line arguments -- return well-known location instead
	switch runtime.GOOS {
	case "darwin", "linux":
		return "/etc/kolide-k2/launcher.flags"
	case "windows":
		return `C:\Program Files\Kolide\Launcher-kolide-k2\conf\launcher.flags`
	default:
		return ""
	}
}

const (
	defaultRootDirectory = "launcher-root"
	skipEnvParse         = runtime.GOOS == "windows" // skip environmental variable parsing on windows
	DefaultTufServer     = "https://tuf.kolide.com"
	DefaultMirror        = "https://dl.kolide.co"
)

// Adapted from
// https://stackoverflow.com/questions/28322997/how-to-get-a-list-of-values-into-a-flag-in-golang/28323276#28323276
type ArrayFlags []string

func (i *ArrayFlags) String() string {
	return strings.Join(*i, " ")
}

func (i *ArrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// ParseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func ParseOptions(subcommandName string, args []string) (*Options, error) {
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
		flCertPins                        = flagset.String("cert_pins", "", "Comma separated, hex encoded SHA256 hashes of pinned subject public key info")
		flControlRequestInterval          = flagset.Duration("control_request_interval", 60*time.Second, "The interval at which the control server requests will be made")
		flEnrollSecret                    = flagset.String("enroll_secret", "", "The enroll secret that is used in your environment")
		flEnrollSecretPath                = flagset.String("enroll_secret_path", "", "Optionally, the path to your enrollment secret")
		flInitialRunner                   = flagset.Bool("with_initial_runner", false, "Run differential queries from config ahead of scheduled interval.")
		flKolideServerURL                 = flagset.String("hostname", "", "The hostname of the gRPC server")
		flKolideHosted                    = flagset.Bool("kolide_hosted", false, "Use Kolide SaaS settings for defaults")
		flTransport                       = flagset.String("transport", "jsonrpc", "The transport protocol that should be used to communicate with remote (default: jsonrpc)")
		flLoggingInterval                 = flagset.Duration("logging_interval", 60*time.Second, "The interval at which logs should be flushed to the server")
		flOsquerydPath                    = flagset.String("osqueryd_path", "", "Path to the osqueryd binary to use (Default: find osqueryd in $PATH)")
		flOsqueryHealthcheckStartupDelay  = flagset.Duration("osquery_healthcheck_startup_delay", 10*time.Minute, "time to wait before beginning osquery healthchecks")
		flWatchdogEnabled                 = flagset.Bool("watchdog_enabled", false, "Whether to enable the osquery watchdog")
		flWatchdogDelaySec                = flagset.Int("watchdog_delay_sec", 120, "How many seconds to delay running watchdog after osquery startup")
		flWatchdogMemoryLimitMB           = flagset.Int("watchdog_memory_limit_mb", 600, "osquery memory utilization limit in MB")
		flWatchdogUtilizationLimitPercent = flagset.Int("watchdog_utilization_limit_percent", 50, "osquery CPU utilization limit in percent")
		flRootDirectory                   = flagset.String("root_directory", DefaultRootDirectoryPath, "The location of the local database, pidfiles, etc.")
		flRootPEM                         = flagset.String("root_pem", "", "Path to PEM file including root certificates to verify against")
		flVersion                         = flagset.Bool("version", false, "Print Launcher version and exit")
		flLogMaxBytesPerBatch             = flagset.Int("log_max_bytes_per_batch", 0, "Maximum size of a batch of logs. Recommend leaving unset, and launcher will determine")
		flOsqueryFlags                    ArrayFlags // set below with flagset.Var
		flConfigFilePath                  = flagset.String("config", DefaultConfigFilePath, "config file to parse options from (optional)")
		flExportTraces                    = flagset.Bool("export_traces", false, "Whether to export traces")
		flTraceSamplingRate               = flagset.Float64("trace_sampling_rate", 0.0, "What fraction of traces should be sampled")
		flLogIngestServerURL              = flagset.String("log_ingest_url", "", "Where to export logs")
		flTraceIngestServerURL            = flagset.String("trace_ingest_url", "", "Where to export traces")
		flDisableIngestTLS                = flagset.Bool("disable_trace_ingest_tls", false, "Disable TLS for observability ingest server communication")

		// Autoupdate options
		flAutoupdate              = flagset.Bool("autoupdate", DefaultAutoupdate, "Whether or not the osquery autoupdater is enabled (default: false)")
		flTufServerURL            = flagset.String("tuf_url", DefaultTufServer, "TUF update server (default: https://tuf.kolide.com)")
		flMirrorURL               = flagset.String("mirror_url", DefaultMirror, "The mirror server for autoupdates (default: https://dl.kolide.co)")
		flAutoupdateInterval      = flagset.Duration("autoupdate_interval", 1*time.Hour, "The interval to check for updates (default: once every hour)")
		flUpdateChannel           = flagset.String("update_channel", "stable", "The channel to pull updates from (options: stable, beta, nightly)")
		flAutoupdateInitialDelay  = flagset.Duration("autoupdater_initial_delay", 1*time.Hour, "Initial autoupdater subprocess delay")
		flUpdateDirectory         = flagset.String("update_directory", "", "Local directory to hold updates for osqueryd and launcher")
		flAutoupdateDownloadSplay = flagset.Duration("autoupdate_download_splay", 8*time.Hour, "duration of time over which launcher should select random delay before downloading")

		// Development & Debugging options
		flDebug                = flagset.Bool("debug", false, "Whether or not debug logging is enabled (default: false)")
		flOsqueryVerbose       = flagset.Bool("osquery_verbose", false, "Enable verbose osqueryd (default: false)")
		flDeveloperUsage       = flagset.Bool("dev_help", false, "Print full Launcher help, including developer options (default: false)")
		flInsecureTransport    = flagset.Bool("insecure_transport", false, "Do not use TLS for transport layer (default: false)")
		flInsecureTLS          = flagset.Bool("insecure", false, "Do not verify TLS certs for outgoing connections (default: false)")
		flIAmBreakingEELicense = flagset.Bool("i-am-breaking-ee-license", false, "Skip license check before running localserver (default: false)")
		flDelayStart           = flagset.Duration("delay_start", 0*time.Second, "How much time to wait before starting launcher")
		flLocalDevelopmentPath = flagset.String("localdev_path", "", "Path to local launcher build")
		flPackageIdentifier    = flagset.String("identifier", DefaultLauncherIdentifier, "packaging identifier used to determine service names, paths, etc. (default: kolide-k2)")

		// deprecated options, kept for any kind of config file compatibility
		_ = flagset.String("debug_log_file", "", "DEPRECATED")
		_ = flagset.Bool("control", false, "DEPRECATED")
		_ = flagset.String("control_hostname", "", "DEPRECATED")
		_ = flagset.Bool("disable_control_tls", false, "DEPRECATED")
		_ = flagset.String("notary_url", "", "DEPRECATED")
		_ = flagset.String("notary_prefix", "", "DEPRECATED")
		_ = flagset.String("config_tls_endpoint", "", "DEPRECATED")
		_ = flagset.String("enroll_tls_endpoint", "", "DEPRECATED")
		_ = flagset.String("logger_tls_endpoint", "", "DEPRECATED")
		_ = flagset.String("distributed_tls_read_endpoint", "", "DEPRECATED")
		_ = flagset.String("distributed_tls_write_endpoint", "", "DEPRECATED")
		_ = flagset.Int64("compactdb-max-tx", 65536, "DEPRECATED") // moved to new flagset inside compactdb command
	)

	flagset.Var(&flOsqueryFlags, "osquery_flag", "Flags to pass to osquery (possibly overriding Launcher defaults)")

	// Deprecated array
	flagset.Var(&ArrayFlags{}, "autoloaded_extension", "DEPRECATED")

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
		return nil, NewInfoCmdError("--version")
	}

	// handle --dev_help
	if *flDeveloperUsage {
		developerUsage(flagset)
		return nil, NewInfoCmdError("--dev_help")
	}

	// if an osqueryd path was not set, it's likely that we want to use the bundled
	// osqueryd path, but if it cannot be found, we will fail back to using an
	// osqueryd found in the path
	osquerydPath := *flOsquerydPath
	if osquerydPath == "" {
		osquerydPath = FindOsquery()
		if osquerydPath == "" {
			return nil, errors.New("could not find osqueryd binary")
		}
	}

	// On windows, we should make sure osquerydPath ends in .exe
	if runtime.GOOS == "windows" && !strings.HasSuffix(osquerydPath, ".exe") {
		osquerydPath = osquerydPath + ".exe"
	}

	if *flEnrollSecret != "" && *flEnrollSecretPath != "" {
		return nil, errors.New("both enroll_secret and enroll_secret_path were defined")
	}

	var updateChannel UpdateChannel
	switch *flUpdateChannel {
	case "", "stable":
		updateChannel = Stable
	case "beta":
		updateChannel = Beta
	case "alpha":
		updateChannel = Alpha
	case "nightly":
		updateChannel = Nightly
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
		*flKolideHosted = true

	case *flKolideServerURL == "k2device-preprod.kolide.com":
		controlServerURL = "k2control-preprod.kolide.com"
		*flKolideHosted = true

	case strings.HasSuffix(*flKolideServerURL, "herokuapp.com"):
		controlServerURL = *flKolideServerURL
		*flKolideHosted = true

	case *flKolideServerURL == "localhost:3443":
		controlServerURL = *flKolideServerURL
		// We don't plumb flRootPEM through to the control server, just disable TLS for now
		insecureControlTLS = true
		*flKolideHosted = true

	case *flKolideServerURL == "localhost:3000" || *flKolideServerURL == "app.kolide.test:80" || *flIAmBreakingEELicense:
		controlServerURL = *flKolideServerURL
		disableControlTLS = true
		*flKolideHosted = true
	}

	if runtime.GOOS == "windows" {
		// check for old root directories before returning the configured option in case we've stomped over with windows MSI install
		updatedRootDirectory := DetermineRootDirectoryOverride(*flRootDirectory, *flKolideServerURL, *flPackageIdentifier)
		if updatedRootDirectory != *flRootDirectory {
			*flRootDirectory = updatedRootDirectory
		}
	}

	opts := &Options{
		Autoupdate:                      *flAutoupdate,
		AutoupdateInterval:              *flAutoupdateInterval,
		AutoupdateInitialDelay:          *flAutoupdateInitialDelay,
		AutoupdateDownloadSplay:         *flAutoupdateDownloadSplay,
		CertPins:                        certPins,
		ConfigFilePath:                  *flConfigFilePath,
		Control:                         false,
		ControlServerURL:                controlServerURL,
		ControlRequestInterval:          *flControlRequestInterval,
		Debug:                           *flDebug,
		DelayStart:                      *flDelayStart,
		DisableControlTLS:               disableControlTLS,
		Identifier:                      *flPackageIdentifier,
		InsecureControlTLS:              insecureControlTLS,
		EnableInitialRunner:             *flInitialRunner,
		WatchdogEnabled:                 *flWatchdogEnabled,
		EnrollSecret:                    *flEnrollSecret,
		EnrollSecretPath:                *flEnrollSecretPath,
		ExportTraces:                    *flExportTraces,
		LogIngestServerURL:              *flLogIngestServerURL,
		LocalDevelopmentPath:            *flLocalDevelopmentPath,
		TraceIngestServerURL:            *flTraceIngestServerURL,
		DisableTraceIngestTLS:           *flDisableIngestTLS,
		IAmBreakingEELicense:            *flIAmBreakingEELicense,
		InsecureTLS:                     *flInsecureTLS,
		InsecureTransport:               *flInsecureTransport,
		KolideHosted:                    *flKolideHosted,
		KolideServerURL:                 *flKolideServerURL,
		LogMaxBytesPerBatch:             *flLogMaxBytesPerBatch,
		LoggingInterval:                 *flLoggingInterval,
		MirrorServerURL:                 *flMirrorURL,
		TufServerURL:                    *flTufServerURL,
		OsqueryFlags:                    flOsqueryFlags,
		OsqueryVerbose:                  *flOsqueryVerbose,
		OsquerydPath:                    osquerydPath,
		OsqueryHealthcheckStartupDelay:  *flOsqueryHealthcheckStartupDelay,
		RootDirectory:                   *flRootDirectory,
		RootPEM:                         *flRootPEM,
		TraceSamplingRate:               *flTraceSamplingRate,
		Transport:                       *flTransport,
		UpdateChannel:                   updateChannel,
		UpdateDirectory:                 *flUpdateDirectory,
		WatchdogDelaySec:                *flWatchdogDelaySec,
		WatchdogMemoryLimitMB:           *flWatchdogMemoryLimitMB,
		WatchdogUtilizationLimitPercent: *flWatchdogUtilizationLimitPercent,
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
	printOpt("mirror_url")
	printOpt("autoupdate_interval")
	printOpt("update_channel")
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

// FindOsquery will attempt to find osquery. We don't much care about
// errors here, either we find it, or we don't.
func FindOsquery() string {
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
		"/usr/local/kolide-k2/osquery.app/Contents/MacOS",
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

// UpdateChannel determines the TUF target for a Updater.
// The Default UpdateChannel is Stable.
type UpdateChannel string

const (
	Stable  UpdateChannel = "stable"
	Alpha   UpdateChannel = "alpha"
	Beta    UpdateChannel = "beta"
	Nightly UpdateChannel = "nightly"
)

func (c UpdateChannel) String() string {
	return string(c)
}

func SanitizeUpdateChannel(value string) string {
	switch UpdateChannel(value) {
	case Stable, Alpha, Beta, Nightly:
		return value
	}
	// Fallback to stable if invalid channel
	return Stable.String()
}

func commandUsage(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "  Usage:\n")
		fmt.Fprintf(os.Stderr, "    %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "  Flags:\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "    --%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}

// IsKolideHostedServerURL is a convenience function to enable gating functionality for
// developer (or other non-production) deployments
func IsKolideHostedServerURL(serverURL string) bool {
	return serverURL == "k2device.kolide.com" || serverURL == "k2device-preprod.kolide.com"
}

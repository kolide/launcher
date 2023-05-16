package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/checkpoint"
	"github.com/peterbourgon/ff/v3"

	"github.com/fatih/color"
	"golang.org/x/exp/slices"
)

var (
	cRunning = color.New(color.FgCyan)
	cHeader  = color.New(color.FgHiWhite).Add(color.Bold)
	cWarn    = color.New(color.FgHiYellow)

	pass = color.New(color.FgGreen).PrintlnFunc()
	fail = color.New(color.FgRed).Add(color.Bold).PrintlnFunc()

	// Println functions for checkup details
	cInfo = color.New(color.FgWhite)
	info  = func(a ...interface{}) {
		cInfo.Println(fmt.Sprintf("    %s", a...))
	}
	warn = func(a ...interface{}) {
		cWarn.Println(fmt.Sprintf("    %s", a...))
	}
	bad = func(a ...interface{}) {
		cInfo.Println(fmt.Sprintf("❌  %s", a...))
	}
	good = func(a ...interface{}) {
		cInfo.Println(fmt.Sprintf("✅  %s", a...))
	}

	configFile string
)

// checkup is
type checkup struct {
	name   string
	check  func() (string, error)
	failed bool
}

func runDoctor(args []string) error {
	logger := log.With(logutil.NewCLILogger(true), "caller", log.DefaultCaller)
	opts, err := parseDoctorOptions(os.Args[2:])
	if err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	cRunning.Println("Kolide launcher doctor version:")
	version.PrintFull()
	cRunning.Println("\nRunning Kolide launcher checkups...")

	checkups := []*checkup{
		{
			name: "Check platform",
			check: func() (string, error) {
				return checkupPlatform(runtime.GOOS)
			},
		},
		{
			name: "Root directory contents",
			check: func() (string, error) {
				return checkupRootDir(getFilepaths(k.RootDirectory(), "*"))
			},
		},
		// TODO: Check application directories for launcher & osquery binaries
		{
			name: "Check communication with Kolide",
			check: func() (string, error) {
				return checkupConnectivity(logger, k)
			},
		},
		{
			name: "Check version",
			check: func() (string, error) {
				return checkupVersion(version.Version())
			},
		},
		{
			name: "Check config file",
			check: func() (string, error) {
				return checkupConfigFile(configFile)
			},
		},
		{
			name: "Check logs",
			check: func() (string, error) {
				return checkupLogFiles(getFilepaths(k.RootDirectory(), "debug*"))
			},
		},
	}

	failedCheckups := []*checkup{}

	// Sequentially run all of the checkups
	for _, c := range checkups {
		c.run()
	}

	if len(failedCheckups) > 0 {
		fail("\nSome checkups failed:")

		for _, c := range failedCheckups {
			fail("%s\n", c.name)
		}
	} else {
		pass("\nAll checkups passed! Your Kolide launcher is healthy.")
	}

	return nil
}

func parseDoctorOptions(args []string) (*launcher.Options, error) {
	flagset := flag.NewFlagSet("launcher doctor", flag.ExitOnError)
	flagset.Usage = func() { usage(flagset) }

	// TODO: Provide platform-specific defaults for files/dirs in case a config file doesn't exist

	var (
		// Primary options
		flAutoloadedExtensions   arrayFlags
		flCertPins               = flagset.String("cert_pins", "", "Comma separated, hex encoded SHA256 hashes of pinned subject public key info")
		flControlRequestInterval = flagset.Duration("control_request_interval", 60*time.Second, "The interval at which the control server requests will be made")
		flEnrollSecret           = flagset.String("enroll_secret", "", "The enroll secret that is used in your environment")
		flEnrollSecretPath       = flagset.String("enroll_secret_path", "", "Optionally, the path to your enrollment secret")
		flInitialRunner          = flagset.Bool("with_initial_runner", false, "Run differential queries from config ahead of scheduled interval.")
		flKolideServerURL        = flagset.String("hostname", "", "The hostname of the gRPC server")
		flKolideHosted           = flagset.Bool("kolide_hosted", true, "Use Kolide SaaS settings for defaults")
		flTransport              = flagset.String("transport", "grpc", "The transport protocol that should be used to communicate with remote (default: grpc)")
		flLoggingInterval        = flagset.Duration("logging_interval", 60*time.Second, "The interval at which logs should be flushed to the server")
		flOsquerydPath           = flagset.String("osqueryd_path", "", "Path to the osqueryd binary to use (Default: find osqueryd in $PATH)")
		flRootDirectory          = flagset.String("root_directory", "", "The location of the local database, pidfiles, etc.")
		flRootPEM                = flagset.String("root_pem", "", "Path to PEM file including root certificates to verify against")
		flVersion                = flagset.Bool("version", false, "Print Launcher version and exit")
		flLogMaxBytesPerBatch    = flagset.Int("log_max_bytes_per_batch", 0, "Maximum size of a batch of logs. Recommend leaving unset, and launcher will determine")
		flOsqueryFlags           arrayFlags // set below with flagset.Var
		flCompactDbMaxTx         = flagset.Int64("compactdb-max-tx", 65536, "Maximum transaction size used when compacting the internal DB")
		flConfigFile             = flagset.String("config", "", "config file to parse options from (optional)")

		// osquery TLS endpoints
		flOsqTlsConfig    = flagset.String("config_tls_endpoint", "", "Config endpoint for the osquery tls transport")
		flOsqTlsEnroll    = flagset.String("enroll_tls_endpoint", "", "Enroll endpoint for the osquery tls transport")
		flOsqTlsLogger    = flagset.String("logger_tls_endpoint", "", "Logger endpoint for the osquery tls transport")
		flOsqTlsDistRead  = flagset.String("distributed_tls_read_endpoint", "", "Distributed read endpoint for the osquery tls transport")
		flOsqTlsDistWrite = flagset.String("distributed_tls_write_endpoint", "", "Distributed write endpoint for the osquery tls transport")

		// Autoupdate options
		flAutoupdate             = flagset.Bool("autoupdate", true, "Whether or not the osquery autoupdater is enabled (default: false)")
		flNotaryServerURL        = flagset.String("notary_url", autoupdate.DefaultNotary, "The Notary update server (default: https://notary.kolide.co)")
		flTufServerURL           = flagset.String("tuf_url", tuf.DefaultTufServer, "TUF update server (default: https://tuf.kolide.com)")
		flMirrorURL              = flagset.String("mirror_url", autoupdate.DefaultMirror, "The mirror server for autoupdates (default: https://dl.kolide.co)")
		flAutoupdateInterval     = flagset.Duration("autoupdate_interval", 1*time.Hour, "The interval to check for updates (default: once every hour)")
		flUpdateChannel          = flagset.String("update_channel", "stable", "The channel to pull updates from (options: stable, beta, nightly)")
		flNotaryPrefix           = flagset.String("notary_prefix", autoupdate.DefaultNotaryPrefix, "The prefix for Notary path that contains the collections (default: kolide/)")
		flAutoupdateInitialDelay = flagset.Duration("autoupdater_initial_delay", 1*time.Hour, "Initial autoupdater subprocess delay")
		flUpdateDirectory        = flagset.String("update_directory", "", "Local directory to hold updates for osqueryd and launcher")

		// Development & Debugging options
		flDebug          = flagset.Bool("debug", false, "Whether or not debug logging is enabled (default: false)")
		flOsqueryVerbose = flagset.Bool("osquery_verbose", false, "Enable verbose osqueryd (default: false)")
		// flDeveloperUsage       = flagset.Bool("dev_help", false, "Print full Launcher help, including developer options (default: false)")
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

	configFile = *flConfigFile

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
		Transport:                          *flTransport,
		UpdateChannel:                      updateChannel,
		UpdateDirectory:                    *flUpdateDirectory,
	}

	return opts, nil
}

func (c *checkup) run() {
	if c.check == nil {
	}

	cRunning.Printf("\nRunning checkup: ")
	cHeader.Printf("%s\n", c.name)

	result, err := c.check()
	if err != nil {
		info(result)
		bad(err)
		fail("𐄂 Checkup failed!")
	} else {
		good(result)
		pass("✔ Checkup passed!")
	}
}

func checkupPlatform(os string) (string, error) {
	if slices.Contains([]string{"windows", "darwin", "linux"}, os) {
		return fmt.Sprintf("Platform: %s", os), nil
	}
	return "", fmt.Errorf("Unsupported platform: %s", os)
}

type launcherFile struct {
	name  string
	found bool
}

func checkupRootDir(filepaths []string) (string, error) {
	importantFiles := []*launcherFile{
		{
			name: "debug.json",
		},
		{
			name: "launcher.db",
		},
		{
			name: "osquery.db",
		},
	}

	if filepaths != nil && len(filepaths) > 0 {
		for _, fp := range filepaths {
			for _, f := range importantFiles {
				if filepath.Base(fp) == f.name {
					f.found = true
				}
			}
		}
	}

	var failures int
	for _, f := range importantFiles {
		if f.found {
			good(f.name)
		} else {
			bad(f.name)
			failures = failures + 1
		}
	}

	if failures == 0 {
		return "Root directory files found", nil
	}

	return "", fmt.Errorf("%d root directory files not found", failures)
}

func checkupConnectivity(logger log.Logger, k types.Knapsack) (string, error) {
	var failures int
	checkpointer := checkpoint.New(logger, k)
	connections := checkpointer.Connections()
	for k, v := range connections {
		if v == "successful tcp connection" {
			good(fmt.Sprintf("%s - %s", k, v))
		} else {
			bad(fmt.Sprintf("%s - %s", k, v))
			failures = failures + 1
		}
	}

	ipLookups := checkpointer.IpLookups()
	for k, v := range ipLookups {
		valStrSlice, ok := v.([]string)
		if ok && len(valStrSlice) > 0 {
			good(fmt.Sprintf("%s - %s", k, valStrSlice))
		} else {
			bad(fmt.Sprintf("%s - %s", k, valStrSlice))
			failures = failures + 1
		}
	}

	notaryVersions, err := checkpointer.NotaryVersions()
	if err != nil {
		bad(fmt.Errorf("could not fetch notary versions: %w", err))
		failures = failures + 1
	}

	for k, v := range notaryVersions {
		if _, err := strconv.ParseInt(v, 10, 32); err == nil {
			good(fmt.Sprintf("%s - %s", k, v))
		} else {
			bad(fmt.Sprintf("%s - %s", k, v))
			failures = failures + 1
		}
	}

	if failures == 0 {
		return "Successfully communicated with Kolide", nil
	}

	return "", fmt.Errorf("%d failures encountered while attempting communication with Kolide", failures)
}

func checkupVersion(v version.Info) (string, error) {
	info(fmt.Sprintf("Current version: %s", v.Version))
	// TODO: Query TUF for latest available version for this launcher instance
	warn(fmt.Sprintf("Target version: %s", "Unknown"))

	// TODO: Choose failure based on current >= target
	if true {
		return "Up to date!", nil
	}

	return "", fmt.Errorf("launcher is out of date")
}

func checkupConfigFile(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("No config file found")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		info(scanner.Text())
	}

	return "Config file found", nil
}

func checkupLogFiles(filepaths []string) (string, error) {
	var foundCurrentLogFile bool
	for _, f := range filepaths {
		filename := filepath.Base(f)
		info(filename)

		if filename == "debug.json" {
			foundCurrentLogFile = true

			fi, err := os.Stat(f)
			if err == nil {
				info("")
				info(fmt.Sprintf("Most recent log file: %s", filename))
				info(fmt.Sprintf("Latest modification: %s", fi.ModTime().String()))
				info(fmt.Sprintf("File size (B): %d", fi.Size()))
			}
		}
	}

	if foundCurrentLogFile {
		return "Log file found", nil
	}
	return "", fmt.Errorf("No log file found")
}

func getFilepaths(elem ...string) []string {
	fileGlob := filepath.Join(elem...)
	filepaths, err := filepath.Glob(fileGlob)

	if err == nil && len(filepaths) > 0 {
		return filepaths
	}

	return nil
}

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/autoupdate"
	"github.com/kolide/launcher/osquery"
	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquery_logger "github.com/kolide/osquery-go/plugin/logger"
	"github.com/kolide/updater/tuf"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	// applicationRoot is the path where the launcher filesystem root is located
	applicationRoot = "/usr/local/kolide/"
	// defaultOsquerydPath is the path to the bundled osqueryd binary
	defaultOsquerydPath = filepath.Join(applicationRoot, "bin/osqueryd")
)

// options is the set of configurable options that may be set when launching this
// program
type options struct {
	kolideServerURL    string
	enrollSecret       string
	enrollSecretPath   string
	rootDirectory      string
	osquerydPath       string
	autoupdate         bool
	printVersion       bool
	debug              bool
	insecureTLS        bool
	insecureGRPC       bool
	notaryServerURL    string
	mirrorServerURL    string
	autoupdateInterval time.Duration
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
		// Primary options
		flRootDirectory = flag.String(
			"root_directory",
			env.String("KOLIDE_LAUNCHER_ROOT_DIRECTORY", os.TempDir()),
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
			"Path to the osqueryd binary to use",
		)

		// Autoupdate options
		flAutoupdate = flag.Bool(
			"autoupdate",
			true,
			"Whether or not the osquery autoupdater is enabled (default: true)",
		)
		flNotaryServerURL = flag.String(
			"notary_url",
			env.String("KOLIDE_LAUNCHER_NOTARY_SERVER_URL", ""),
			"The Notary update server (default: https://notary.kolide.com)",
		)
		flMirrorURL = flag.String(
			"mirror_url",
			env.String("KOLIDE_LAUNCHER_MIRROR_SERVER_URL", ""),
			"The mirror server for autoupdates (default: https://dl.kolide.com)",
		)
		flAutoupdateInterval = flag.Duration(
			"autoupdate_interval",
			duration("KOLIDE_LAUNCHER_AUTOUPDATE_INTERVAL", 1*time.Hour),
			"The interval to check for updates (default: once every hour)",
		)

		// Development options
		flDebug = flag.Bool(
			"debug",
			false,
			"Whether or not debug logging is enabled (default: false)",
		)
		flInsecureTLS = flag.Bool(
			"insecure",
			false,
			"Do not verify TLS certs for outgoing connections (default: false)",
		)
		flInsecureGRPC = flag.Bool(
			"insecure_grpc",
			false,
			"Dial GRPC without a TLS config (default: false)",
		)

		// Version command: launcher --version
		flVersion = flag.Bool(
			"version",
			false,
			"Print Launcher version and exit",
		)
	)

	flag.Usage = func() {
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
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "  Additionally, all options can be set as environment variables using the following convention:\n")
		fmt.Fprintf(os.Stderr, "      KOLIDE_LAUNCHER_OPTION=value launcher\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Development Options:\n")
		fmt.Fprintf(os.Stderr, "\n")
		printOpt("debug")
		fmt.Fprintf(os.Stderr, "\n")
		printOpt("insecure")
		printOpt("insecure_grpc")
		fmt.Fprintf(os.Stderr, "\n")
		printOpt("notary_url")
		printOpt("mirror_url")
		printOpt("autoupdate_interval")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "For more information, check out https://kolide.com/osquery\n")
		fmt.Fprintf(os.Stderr, "\n")
	}

	flag.Parse()

	opts := &options{
		kolideServerURL:    *flKolideServerURL,
		enrollSecret:       *flEnrollSecret,
		enrollSecretPath:   *flEnrollSecretPath,
		rootDirectory:      *flRootDirectory,
		osquerydPath:       *flOsquerydPath,
		autoupdate:         *flAutoupdate,
		printVersion:       *flVersion,
		debug:              *flDebug,
		insecureTLS:        *flInsecureTLS,
		insecureGRPC:       *flInsecureGRPC,
		notaryServerURL:    *flNotaryServerURL,
		mirrorServerURL:    *flMirrorURL,
		autoupdateInterval: *flAutoupdateInterval,
	}

	// if an osqueryd path was not set, it's likely that we want to use the bundled
	// osqueryd path, but if it cannot be found, we will fail back to using an
	// osqueryd found in the path
	if opts.osquerydPath == "" {
		if _, err := os.Stat(defaultOsquerydPath); err == nil {
			opts.osquerydPath = defaultOsquerydPath
		} else if path, err := exec.LookPath("osqueryd"); err == nil {
			opts.osquerydPath = path
		} else {
			return nil, errors.New("Could not find osqueryd binary")
		}
	}

	if opts.enrollSecret != "" && opts.enrollSecretPath != "" {
		return nil, errors.New("Both enroll_secret and enroll_secret_path were defined")
	}

	return opts, nil
}

func insecureHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}

// We have to find the instance of osqueryd and send it an interrupt so it
// shuts down it's extensions which are child processes of osqueryd. If we
// don't do this the extension continues to run and osqueryd thinks we're trying
// to register a duplicate extension and start up of new launcher process fails.
func shutdownOsquery(rootdir string) error {
	pidFilePath := filepath.Join(rootdir, "osquery.pid")
	sPid, err := ioutil.ReadFile(pidFilePath)
	if err != nil {
		return errors.Wrap(err, "finding osquery pid")
	}
	pid, err := strconv.Atoi(string(sPid))
	if err != nil {
		return errors.Wrap(err, "converting pid to int")
	}
	sigTerm := syscall.Signal(15)
	if err := syscall.Kill(pid, sigTerm); err != nil {
		return errors.Wrap(err, "killing osqueryd")
	}
	time.Sleep(5 * time.Second)
	return nil
}

func enableAutoUpdate(
	notaryURL, mirrorURL, binaryPath, rootDirectory string,
	autoupdateInterval time.Duration,
	restart func() error,
	client *http.Client,
	logger log.Logger,
) (stop func(), err error) {
	autoupdateOpts := []autoupdate.UpdaterOption{
		autoupdate.WithHTTPClient(client),
		autoupdate.WithNotaryURL(notaryURL),
		autoupdate.WithLogger(logger),
	}
	if mirrorURL != "" {
		autoupdateOpts = append(autoupdateOpts, autoupdate.WithMirrorURL(mirrorURL))
	}

	var osquerydUpdaterOpts []autoupdate.UpdaterOption
	osquerydUpdaterOpts = append(osquerydUpdaterOpts, autoupdateOpts...)
	osquerydUpdaterOpts = append(osquerydUpdaterOpts, autoupdate.WithFinalizer(restart))
	osquerydUpdater, err := autoupdate.NewUpdater(
		binaryPath,
		rootDirectory,
		logger,
		osquerydUpdaterOpts...,
	)
	if err != nil {
		return nil, err
	}
	stopO, err := osquerydUpdater.Run(tuf.WithFrequency(autoupdateInterval))
	if err != nil {
		return nil, err
	}

	launcherPath, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "get launcher path")
	}

	// call this method to restart the launcher when autoupdate completes.
	launcherFinalizer := func() error {

		if err = shutdownOsquery(rootDirectory); err != nil {
			level.Warn(logger).Log(
				"method", "launcherFinalizer",
				"err", err,
			)
		}
		// replace launcher
		if err = syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
			return errors.Wrap(err, "restarting launcher")
		}
		return nil
	}

	var launcherUpdaterOpts []autoupdate.UpdaterOption
	launcherUpdaterOpts = append(launcherUpdaterOpts, autoupdateOpts...)
	launcherUpdaterOpts = append(launcherUpdaterOpts, autoupdate.WithFinalizer(launcherFinalizer))
	launcherUpdater, err := autoupdate.NewUpdater(
		launcherPath,
		rootDirectory,
		logger,
		launcherUpdaterOpts...,
	)
	if err != nil {
		return nil, err
	}
	stopL, err := launcherUpdater.Run(tuf.WithFrequency(autoupdateInterval))
	if err != nil {
		return nil, err
	}

	return func() {
		stopO()
		stopL()
	}, nil
}

func logFatal(logger log.Logger, args ...interface{}) {
	level.Info(logger).Log(args...)
	os.Exit(1)
}

// dialGRPC creates a grpc client connection.
func dialGRPC(
	serverURL string,
	insecureTLS bool,
	insecureGRPC bool,
	logger log.Logger,
) (*grpc.ClientConn, error) {
	level.Info(logger).Log(
		"msg", "dialing grpc server",
		"server", serverURL,
		"tls_secure", insecureTLS == false,
		"grpc_secure", insecureGRPC == false,
	)
	grpcOpts := []grpc.DialOption{
		grpc.WithTimeout(time.Second),
	}
	if insecureGRPC {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	} else {
		host, _, err := net.SplitHostPort(serverURL)
		if err != nil {
			return nil, errors.Wrapf(err, "split grpc server host and port: %s", serverURL)
		}
		creds := credentials.NewTLS(&tls.Config{
			ServerName:         host,
			InsecureSkipVerify: insecureTLS,
		})
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))
	}
	conn, err := grpc.Dial(
		serverURL,
		grpcOpts...,
	)
	return conn, err
}

func main() {
	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	opts, err := parseOptions()
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "invalid options"))
	}

	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	if opts.debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	// Note: caller must be added after everything else that decorates the
	// logger (otherwise we get incorrect line numbers).
	logger = log.With(logger, "caller", log.DefaultCaller)

	if _, err := osquery.DetectPlatform(); err != nil {
		logFatal(logger, "err", errors.Wrap(err, "detecting platform"))
	}

	httpClient := http.DefaultClient
	if opts.insecureTLS {
		httpClient = insecureHTTPClient()
	}

	versionInfo := version.Version()
	level.Info(logger).Log("msg", "started kolide launcher", "version", versionInfo.Version, "build", versionInfo.Revision)

	db, err := bolt.Open(filepath.Join(opts.rootDirectory, "launcher.db"), 0600, nil)
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "open local store"))
	}
	defer db.Close()

	conn, err := dialGRPC(opts.kolideServerURL, opts.insecureTLS, opts.insecureGRPC, logger)
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "dialing grpc server"))
	}
	defer conn.Close()

	client := service.New(conn, logger)

	var enrollSecret string
	if opts.enrollSecret != "" {
		enrollSecret = opts.enrollSecret
	} else if opts.enrollSecretPath != "" {
		content, err := ioutil.ReadFile(opts.enrollSecretPath)
		if err != nil {
			logFatal(logger, "err", errors.Wrap(err, "could not read enroll_secret_path"), "enroll_secret_path", opts.enrollSecretPath)
		}
		enrollSecret = string(content)
	}

	extOpts := osquery.ExtensionOpts{
		EnrollSecret: enrollSecret,
		Logger:       logger,
	}
	ext, err := osquery.NewExtension(client, db, extOpts)
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "starting grpc extension"))
	}

	_, invalid, err := ext.Enroll(context.Background())
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "enrolling host"))
	}
	if invalid {
		logFatal(logger, errors.Wrap(err, "invalid enroll secret"))
	}
	ext.Start()
	defer ext.Shutdown()

	instance, err := osquery.LaunchOsqueryInstance(
		osquery.WithOsquerydBinary(opts.osquerydPath),
		osquery.WithRootDirectory(opts.rootDirectory),
		osquery.WithConfigPluginFlag("kolide_grpc"),
		osquery.WithLoggerPluginFlag("kolide_grpc"),
		osquery.WithDistributedPluginFlag("kolide_grpc"),
		osquery.WithOsqueryExtensionPlugin(config.NewPlugin("kolide_grpc", ext.GenerateConfigs)),
		osquery.WithOsqueryExtensionPlugin(osquery_logger.NewPlugin("kolide_grpc", ext.LogString)),
		osquery.WithOsqueryExtensionPlugin(distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)),
		osquery.WithStdout(os.Stdout),
		osquery.WithStderr(os.Stderr),
		osquery.WithRetries(3),
	)
	if err != nil {
		logFatal(logger, errors.Wrap(err, "launching osquery instance"))
	}

	if opts.notaryServerURL != "" {
		stop, err := enableAutoUpdate(
			opts.notaryServerURL,
			opts.mirrorServerURL,
			opts.osquerydPath,
			opts.rootDirectory,
			opts.autoupdateInterval,
			instance.Restart,
			httpClient,
			logger,
		)
		if err != nil {
			logFatal(logger, errors.Wrap(err, "starting autoupdater"))
		}
		defer stop()
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
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

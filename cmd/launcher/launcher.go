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
	osquerydPath       string
	rootDirectory      string
	notaryServerURL    string
	mirrorServerURL    string
	kolideServerURL    string
	enrollSecret       string
	enrollSecretPath   string
	autoupdateInterval time.Duration
	insecureTLS        bool
	insecureGRPC       bool
	printVersion       bool
	debug              bool
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
		flDebug = flag.Bool(
			"debug",
			false,
			"enable debug logging",
		)
		flVersion = flag.Bool(
			"version",
			false,
			"print launcher version and exit",
		)
		flInsecureTLS = flag.Bool(
			"insecure",
			false,
			"do not verify TLS certs for outgoing connections",
		)
		flInsecureGRPC = flag.Bool(
			"insecure_grpc",
			false,
			"dial GRPC without a TLS config",
		)
		flOsquerydPath = flag.String(
			"osqueryd_path",
			env.String("KOLIDE_LAUNCHER_OSQUERYD_PATH", ""),
			"path to osqueryd binary",
		)
		flRootDirectory = flag.String(
			"root_directory",
			env.String("KOLIDE_LAUNCHER_ROOT_DIRECTORY", os.TempDir()),
			"path to the working directory where file artifacts can be stored",
		)
		flNotaryServerURL = flag.String(
			"notary_url",
			env.String("KOLIDE_LAUNCHER_NOTARY_SERVER_URL", ""),
			"The URL of the notary update server",
		)
		flKolideServerURL = flag.String(
			"hostname",
			env.String("KOLIDE_LAUNCHER_HOSTNAME", ""),
			"Hostname of the remote server to communicate with",
		)
		flEnrollSecret = flag.String(
			"enroll_secret",
			env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", ""),
			"The enrollment secret used to authenticate with the server",
		)
		flEnrollSecretPath = flag.String(
			"enroll_secret_path",
			env.String("KOLIDE_LAUNCHER_ENROLL_SECRET_PATH", ""),
			"Path to a file containing the enrollment secret",
		)
		flMirrorURL = flag.String(
			"mirror_url",
			env.String("KOLIDE_LAUNCHER_MIRROR_SERVER_URL", ""),
			"The URL of the mirror server for autoupdates",
		)
		flAutoupdateInterval = flag.Duration(
			"autoupdate_interval",
			duration("KOLIDE_LAUNCHER_AUTOUPDATE_INTERVAL", 1*time.Hour),
			"The interval when launcher checks for new updates. Only enabled if notary_url is set.",
		)
	)
	flag.Parse()

	opts := &options{
		osquerydPath:       *flOsquerydPath,
		rootDirectory:      *flRootDirectory,
		notaryServerURL:    *flNotaryServerURL,
		mirrorServerURL:    *flMirrorURL,
		printVersion:       *flVersion,
		kolideServerURL:    *flKolideServerURL,
		enrollSecret:       *flEnrollSecret,
		enrollSecretPath:   *flEnrollSecretPath,
		debug:              *flDebug,
		insecureTLS:        *flInsecureTLS,
		insecureGRPC:       *flInsecureGRPC,
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

func enableAutoUpdate(
	notaryURL, mirrorURL, binaryPath, rootdir string,
	autoupdateInterval time.Duration,
	restart func() error,
	client *http.Client,
	logger log.Logger,
) (stop func(), err error) {
	defaultOpts := []autoupdate.UpdaterOption{
		autoupdate.WithHTTPClient(client),
		autoupdate.WithNotaryURL(notaryURL),
		autoupdate.WithLogger(logger),
	}
	if mirrorURL != "" {
		defaultOpts = append(defaultOpts, autoupdate.WithMirrorURL(mirrorURL))
	}

	var osquerydUpdaterOpts []autoupdate.UpdaterOption
	osquerydUpdaterOpts = append(osquerydUpdaterOpts, defaultOpts...)
	osquerydUpdaterOpts = append(osquerydUpdaterOpts, autoupdate.WithFinalizer(restart))
	osquerydUpdater, err := autoupdate.NewUpdater(
		autoupdate.Destination(binaryPath),
		rootdir,
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
		if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
			return errors.Wrap(err, "restarting launcher")
		}
		return nil
	}

	var launcherUpdaterOpts []autoupdate.UpdaterOption
	launcherUpdaterOpts = append(launcherUpdaterOpts, defaultOpts...)
	launcherUpdaterOpts = append(launcherUpdaterOpts, autoupdate.WithFinalizer(launcherFinalizer))
	launcherUpdater, err := autoupdate.NewUpdater(
		autoupdate.Destination(launcherPath),
		rootdir,
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

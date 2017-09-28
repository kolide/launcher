package main

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/autoupdate"
	"github.com/kolide/launcher/osquery"
	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquery_logger "github.com/kolide/osquery-go/plugin/logger"
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

func logFatal(logger log.Logger, args ...interface{}) {
	level.Info(logger).Log(args...)
	os.Exit(1)
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

	if opts.developerUsage {
		developerUsage()
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

	rootDirectory := opts.rootDirectory
	if rootDirectory == "" {
		rootDirectory = filepath.Join(os.TempDir(), defaultRootDirectory)
		level.Info(logger).Log(
			"msg", "using default system root directory",
			"path", rootDirectory,
		)
	}

	if _, err := osquery.DetectPlatform(); err != nil {
		logFatal(logger, "err", errors.Wrap(err, "detecting platform"))
	}

	httpClient := http.DefaultClient
	if opts.insecureTLS {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	versionInfo := version.Version()
	level.Info(logger).Log("msg", "started kolide launcher", "version", versionInfo.Version, "build", versionInfo.Revision)

	db, err := bolt.Open(filepath.Join(rootDirectory, "launcher.db"), 0600, nil)
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

	// Start the osqueryd instance
	instance, err := osquery.LaunchOsqueryInstance(
		osquery.WithOsquerydBinary(opts.osquerydPath),
		osquery.WithRootDirectory(rootDirectory),
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

	// If the autoupdater is enabled, enable it for both osquery and launcher
	if opts.autoupdate {
		enabler := &updateEnabler{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: opts.autoupdateInterval,
			NotaryURL:          opts.notaryServerURL,
			MirrorURL:          opts.mirrorServerURL,
			HttpClient:         httpClient,
		}

		stopOsquery, err := enabler.EnableBinary(
			opts.osquerydPath,
			autoupdate.WithFinalizer(instance.Restart),
			autoupdate.WithUpdateChannel(opts.updateChannel),
		)
		if err != nil {
			logFatal(logger, err)
		}
		defer stopOsquery()

		launcherPath, err := os.Executable()
		if err != nil {
			logFatal(logger, err)
		}
		stopLauncher, err := enabler.EnableBinary(
			launcherPath,
			autoupdate.WithFinalizer(launcherFinalizer(logger, rootDirectory)),
			autoupdate.WithUpdateChannel(opts.updateChannel),
		)
		if err != nil {
			logFatal(logger, err)
		}
		defer stopLauncher()
	}

	// Wait forever
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
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

func launcherFinalizer(logger log.Logger, rootDirectory string) func() error {
	return func() error {
		if err := shutdownOsquery(rootDirectory); err != nil {
			level.Warn(logger).Log(
				"method", "launcherFinalizer",
				"err", err,
			)
		}
		// replace launcher
		if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
			return errors.Wrap(err, "restarting launcher")
		}
		return nil
	}
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

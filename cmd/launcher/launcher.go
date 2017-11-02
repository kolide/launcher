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
	"strings"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/autoupdate"
	"github.com/kolide/launcher/debug"
	kolidelog "github.com/kolide/launcher/log"
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

func main() {
	logger := kolidelog.NewLogger(os.Stderr)
	opts, err := parseOptions()
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "invalid options"))
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
		logger.AllowDebug()
	}
	debug.AttachLogToggle(logger, opts.debug)

	rootDirectory := opts.rootDirectory
	if rootDirectory == "" {
		rootDirectory = filepath.Join(os.TempDir(), defaultRootDirectory)
		if _, err := os.Stat(rootDirectory); os.IsNotExist(err) {
			if err := os.Mkdir(rootDirectory, fs.DirMode); err != nil {
				logger.Fatal("err", errors.Wrap(err, "creating temporary root directory"))
			}
		}
		level.Info(logger).Log(
			"msg", "using default system root directory",
			"path", rootDirectory,
		)
	}

	if err := os.MkdirAll(rootDirectory, 0700); err != nil {
		logger.Fatal("err", errors.Wrap(err, "creating root directory"))
	}

	if _, err := osquery.DetectPlatform(); err != nil {
		logger.Fatal("err", errors.Wrap(err, "detecting platform"))
	}

	debugAddrPath := filepath.Join(rootDirectory, "debug_addr")
	debug.AttachDebugHandler(debugAddrPath, logger)
	defer os.Remove(debugAddrPath)

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
	level.Info(logger).Log(
		"msg", "started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	db, err := bolt.Open(filepath.Join(rootDirectory, "launcher.db"), 0600, nil)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "open local store"))
	}
	defer db.Close()

	conn, err := dialGRPC(opts.kolideServerURL, opts.insecureTLS, opts.insecureGRPC, logger)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "dialing grpc server"))
	}
	defer conn.Close()

	client := service.New(conn, level.Debug(logger))

	var enrollSecret string
	if opts.enrollSecret != "" {
		enrollSecret = opts.enrollSecret
	} else if opts.enrollSecretPath != "" {
		content, err := ioutil.ReadFile(opts.enrollSecretPath)
		if err != nil {
			logger.Fatal("err", errors.Wrap(err, "could not read enroll_secret_path"), "enroll_secret_path", opts.enrollSecretPath)
		}
		enrollSecret = string(content)
	}

	extOpts := osquery.ExtensionOpts{
		EnrollSecret: enrollSecret,
		Logger:       logger,
	}
	ext, err := osquery.NewExtension(client, db, extOpts)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "starting grpc extension"))
	}

	_, invalid, err := ext.Enroll(context.Background())
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "enrolling host"))
	}
	if invalid {
		logger.Fatal(errors.Wrap(err, "invalid enroll secret"))
	}
	ext.Start()
	defer ext.Shutdown()

	osqueryLogger := &kolidelog.OsqueryLogAdapter{
		level.Debug(log.With(logger, "component", "osquery")),
	}

	// Start the osqueryd instance
	runner, err := osquery.LaunchInstance(
		osquery.WithOsquerydBinary(opts.osquerydPath),
		osquery.WithRootDirectory(rootDirectory),
		osquery.WithConfigPluginFlag("kolide_grpc"),
		osquery.WithLoggerPluginFlag("kolide_grpc"),
		osquery.WithDistributedPluginFlag("kolide_grpc"),
		osquery.WithOsqueryExtensionPlugin(config.NewPlugin("kolide_grpc", ext.GenerateConfigs)),
		osquery.WithOsqueryExtensionPlugin(osquery_logger.NewPlugin("kolide_grpc", ext.LogString)),
		osquery.WithOsqueryExtensionPlugin(distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)),
		osquery.WithStdout(osqueryLogger),
		osquery.WithStderr(osqueryLogger),
		osquery.WithLogger(logger),
	)
	if err != nil {
		logger.Fatal(errors.Wrap(err, "launching osquery instance"))
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
			autoupdate.WithFinalizer(runner.Restart),
			autoupdate.WithUpdateChannel(opts.updateChannel),
		)
		if err != nil {
			logger.Fatal(err)
		}
		defer stopOsquery()

		launcherPath, err := os.Executable()
		if err != nil {
			logger.Fatal(err)
		}
		stopLauncher, err := enabler.EnableBinary(
			launcherPath,
			autoupdate.WithFinalizer(launcherFinalizer(logger, rootDirectory)),
			autoupdate.WithUpdateChannel(opts.updateChannel),
		)
		if err != nil {
			logger.Fatal(err)
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
			level.Info(logger).Log(
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

// tempErr is a wrapper for errors that are "temporary" and should be retried
// for gRPC.
type tempErr struct {
	error
}

func (t tempErr) Temporary() bool {
	return true
}

// tlsCreds overwrites the ClientHandshake method for specific error handling.
type tlsCreds struct {
	credentials.TransportCredentials
}

// ClientHandshake wraps the normal gRPC ClientHandshake, but treats a
// certificate with the wrong name as a temporary rather than permanent error.
// This is important for reconnecting to the gRPC server after, for example,
// the certificate being MitMed by a captive portal (without this, gRPC calls
// will error and never attempt to reconnect).
// See https://github.com/grpc/grpc-go/issues/1571.
func (t *tlsCreds) ClientHandshake(ctx context.Context, s string, c net.Conn) (net.Conn, credentials.AuthInfo, error) {
	conn, info, err := t.TransportCredentials.ClientHandshake(ctx, s, c)
	if err != nil && strings.Contains(err.Error(), "x509: certificate is valid for ") {
		err = &tempErr{err}
	}

	return conn, info, err
}

// dialGRPC creates a grpc client connection.
func dialGRPC(
	serverURL string,
	insecureTLS bool,
	insecureGRPC bool,
	logger log.Logger,
	opts ...grpc.DialOption, // Used for overrides in testing
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
		creds := &tlsCreds{credentials.NewTLS(&tls.Config{
			ServerName:         host,
			InsecureSkipVerify: insecureTLS,
		})}
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))
	}

	grpcOpts = append(grpcOpts, opts...)

	conn, err := grpc.Dial(serverURL, grpcOpts...)
	return conn, err
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/pkg/augeas"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/launcher"
	kolidelog "github.com/kolide/launcher/pkg/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	ktable "github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/launcher/pkg/service"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	osquerylogger "github.com/osquery/osquery-go/plugin/logger"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"golang.org/x/time/rate"
)

const (
	runnerStartTimeout  = 5 * time.Minute  // Total time to wait opening the osquery socket
	runnerStartInterval = 20 * time.Second // how long to wait between attempts to open osquery socket
)

// TODO: the extension, runtime, and client are all kind of entangled
// here. Untangle the underlying libraries and separate into units
func createExtensionRuntime(ctx context.Context, db *bbolt.DB, launcherClient service.KolideService, opts *launcher.Options) (
	run *actor.Actor,
	restart func() error, // restart osqueryd runner
	shutdown func() error, // shutdown osqueryd runner
	err error,
) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

	// read the enroll secret, if either it or the path has been specified
	var enrollSecret string
	if opts.EnrollSecret != "" {
		enrollSecret = opts.EnrollSecret
	} else if opts.EnrollSecretPath != "" {
		content, err := ioutil.ReadFile(opts.EnrollSecretPath)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "could not read enroll_secret_path: %s", opts.EnrollSecretPath)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	}

	// create the osquery extension
	extOpts := osquery.ExtensionOpts{
		EnrollSecret:                      enrollSecret,
		Logger:                            logger,
		LoggingInterval:                   opts.LoggingInterval,
		RunDifferentialQueriesImmediately: opts.EnableInitialRunner,
	}

	// Setting MaxBytesPerBatch is a tradeoff. If it's too low, we
	// can never send a large result. But if it's too high, we may
	// not be able to send the data over a low bandwidth
	// connection before the connection is timed out.
	//
	// The logic for setting this is spread out. The underlying
	// extension defaults to 3mb, to support GRPC's hardcoded 4MB
	// limit. But as we're transport aware here. we can set it to
	// 5MB for others.
	if opts.LogMaxBytesPerBatch != 0 {
		if opts.Transport == "grpc" && opts.LogMaxBytesPerBatch > 3 {
			level.Info(logger).Log(
				"msg", "LogMaxBytesPerBatch is set above the grpc recommended maximum of 3. Expect errors",
				"LogMaxBytesPerBatch", opts.LogMaxBytesPerBatch,
			)
		}
		extOpts.MaxBytesPerBatch = opts.LogMaxBytesPerBatch << 20
	} else if opts.Transport == "grpc" {
		extOpts.MaxBytesPerBatch = 3 << 20
	} else if opts.Transport != "grpc" {
		extOpts.MaxBytesPerBatch = 5 << 20
	}

	// create the extension
	ext, err := osquery.NewExtension(launcherClient, db, extOpts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "starting grpc extension")
	}

	var runnerOptions []runtime.OsqueryInstanceOption

	if opts.Transport == "osquery" {
		var err error
		runnerOptions, err = osqueryRunnerOptions(logger, db, opts)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "creating osquery runner options")
		}
	} else {
		runnerOptions = grpcRunnerOptions(logger, db, opts, ext)
	}

	runner := runtime.LaunchUnstartedInstance(runnerOptions...)

	restartFunc := func() error {
		level.Debug(logger).Log(
			"caller", log.DefaultCaller,
			"msg", "restart function",
		)

		return runner.Restart()
	}

	return &actor.Actor{
			// and the methods for starting and stopping the extension
			Execute: func() error {

				// Start the osqueryd instance
				if err := runner.Start(); err != nil {
					return errors.Wrap(err, "launching osquery instance")
				}

				// Because the runner starts a bunch
				// of async threads, we need to make
				// sure it's healthy before we
				// continue with startup.
				deadlineCtx, cancel := context.WithTimeout(context.Background(), runnerStartTimeout)
				defer cancel()
				limiter := rate.NewLimiter(rate.Every(runnerStartInterval), 1)
				for {
					level.Debug(logger).Log("msg", "Health checks on runner")

					runnerErr := runner.Healthy()

					if runnerErr == nil {
						break
					}

					// Did we timeout? If so, send the error from the healthcheck
					if limiter.Wait(deadlineCtx) != nil {
						level.Debug(logger).Log("msg", "Exiting because runner not healthy", "err", err)
						return errors.Wrapf(runnerErr, "timeout waiting for runner to be healthy")
					}
				}
				level.Debug(logger).Log("msg", "Runner healthy. Moving on")

				// The runner allows querying the osqueryd instance from the extension.
				// Used by the Enroll method below to get initial enrollment details.
				ext.SetQuerier(runner)

				// enroll this launcher with the server
				_, invalid, err := ext.Enroll(ctx)
				if err != nil {
					return errors.Wrap(err, "enrolling host")
				}
				if invalid {
					return errors.Wrap(err, "invalid enroll secret")
				}

				// start the extension
				ext.Start()

				level.Info(logger).Log("msg", "extension started")

				// TODO: remove when underlying libs are refactored
				// everything exits right now, so block this actor on the context finishing
				<-ctx.Done()
				return nil
			},
			Interrupt: func(err error) {
				level.Info(logger).Log("msg", "extension interrupted", "err", err)
				level.Debug(logger).Log("msg", "extension interrupted", "err", err, "stack", fmt.Sprintf("%+v", err))
				ext.Shutdown()
				if runner != nil {
					if err := runner.Shutdown(); err != nil {
						level.Info(logger).Log("msg", "error shutting down runtime", "err", err)
						level.Debug(logger).Log("msg", "error shutting down runtime", "err", err, "stack", fmt.Sprintf("%+v", err))
					}
				}
			},
		},
		restartFunc,
		runner.Shutdown,
		nil
}

// commonRunnerOptions returns osquery runtime options common to all transports
func commonRunnerOptions(logger log.Logger, db *bbolt.DB, opts *launcher.Options) []runtime.OsqueryInstanceOption {
	// create the logging adapters for osquery
	osqueryStderrLogger := kolidelog.NewOsqueryLogAdapter(
		logger,
		kolidelog.WithLevelFunc(level.Info),
		kolidelog.WithKeyValue("component", "osquery"),
		kolidelog.WithKeyValue("osqlevel", "stderr"),
	)
	osqueryStdoutLogger := kolidelog.NewOsqueryLogAdapter(
		logger,
		kolidelog.WithLevelFunc(level.Debug),
		kolidelog.WithKeyValue("component", "osquery"),
		kolidelog.WithKeyValue("osqlevel", "stdout"),
	)

	return []runtime.OsqueryInstanceOption{
		runtime.WithOsquerydBinary(opts.OsquerydPath),
		runtime.WithRootDirectory(opts.RootDirectory),
		runtime.WithOsqueryExtensionPlugins(ktable.LauncherTables(db, opts)...),
		runtime.WithStdout(osqueryStdoutLogger),
		runtime.WithStderr(osqueryStderrLogger),
		runtime.WithLogger(logger),
		runtime.WithOsqueryVerbose(opts.OsqueryVerbose),
		runtime.WithOsqueryFlags(opts.OsqueryFlags),
		runtime.WithAugeasLensFunction(augeas.InstallLenses),
	}
}

// osqueryRunnerOptions returns the osquery runtime options when using native osquery transport
func osqueryRunnerOptions(logger log.Logger, db *bbolt.DB, opts *launcher.Options) ([]runtime.OsqueryInstanceOption, error) {
	// As osquery requires TLS server certs, we'll  use our embedded defaults if not specified
	caCertFile := opts.RootPEM
	if caCertFile == "" {
		var err error
		caCertFile, err = internal.InstallCaCerts(opts.RootDirectory)
		if err != nil {
			return nil, errors.Wrap(err, "writing CA certs")
		}
	}

	runtimeOptions := append(
		commonRunnerOptions(logger, db, opts),
		runtime.WithConfigPluginFlag("tls"),
		runtime.WithDistributedPluginFlag("tls"),
		runtime.WithLoggerPluginFlag("tls"),
		runtime.WithTlsConfigEndpoint(opts.OsqueryTlsConfigEndpoint),
		runtime.WithTlsDistributedReadEndpoint(opts.OsqueryTlsDistributedReadEndpoint),
		runtime.WithTlsDistributedWriteEndpoint(opts.OsqueryTlsDistributedWriteEndpoint),
		runtime.WithTlsEnrollEndpoint(opts.OsqueryTlsEnrollEndpoint),
		runtime.WithTlsHostname(opts.KolideServerURL),
		runtime.WithTlsLoggerEndpoint(opts.OsqueryTlsLoggerEndpoint),
		runtime.WithTlsServerCerts(caCertFile),
	)

	// Enroll secrets... Either we pass a file, or we write a
	// secret, and pass _that_ file
	if opts.EnrollSecretPath != "" {
		runtimeOptions = append(runtimeOptions, runtime.WithEnrollSecretPath(opts.EnrollSecretPath))
	} else if opts.EnrollSecret != "" {
		filename := filepath.Join(opts.RootDirectory, "secret")
		os.WriteFile(filename, []byte(opts.EnrollSecret), 0400)
		runtimeOptions = append(runtimeOptions, runtime.WithEnrollSecretPath(filename))
	}

	return runtimeOptions, nil
}

// grpcRunnerOptions returns the osquery runtime options when using launcher transports. (Eg: grpc or jsonrpc)
func grpcRunnerOptions(logger log.Logger, db *bbolt.DB, opts *launcher.Options, ext *osquery.Extension) []runtime.OsqueryInstanceOption {
	return append(
		commonRunnerOptions(logger, db, opts),
		runtime.WithConfigPluginFlag("kolide_grpc"),
		runtime.WithLoggerPluginFlag("kolide_grpc"),
		runtime.WithDistributedPluginFlag("kolide_grpc"),
		runtime.WithOsqueryExtensionPlugins(
			config.NewPlugin("kolide_grpc", ext.GenerateConfigs),
			distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults),
			osquerylogger.NewPlugin("kolide_grpc", ext.LogString),
		),
	)
}

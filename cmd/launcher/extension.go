package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/pkg/agent/types"
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
)

// actorQuerier is a type wrapper over kolide/kit/actor. This should
// probably all be refactored into reasonable interfaces. But that's
// going to be pretty extensive work.
type actorQuerier struct {
	actor.Actor
	querier func(query string) ([]map[string]string, error)
}

func (aq actorQuerier) Query(query string) ([]map[string]string, error) {
	return aq.querier(query)
}

// TODO: the extension, runtime, and client are all kind of entangled
// here. Untangle the underlying libraries and separate into units
func createExtensionRuntime(ctx context.Context, ktx *types.Knapsack, launcherClient service.KolideService, opts *launcher.Options) (
	run *actorQuerier,
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
		content, err := os.ReadFile(opts.EnrollSecretPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("could not read enroll_secret_path: %s: %w", opts.EnrollSecretPath, err)
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
	ext, err := osquery.NewExtension(launcherClient, ktx, extOpts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("starting grpc extension: %w", err)
	}

	var runnerOptions []runtime.OsqueryInstanceOption

	if opts.Transport == "osquery" {
		var err error
		runnerOptions, err = osqueryRunnerOptions(logger, ktx, opts)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("creating osquery runner options: %w", err)
		}
	} else {
		runnerOptions = grpcRunnerOptions(logger, ktx, opts, ext)
	}

	runner := runtime.LaunchUnstartedInstance(runnerOptions...)

	restartFunc := func() error {
		level.Debug(logger).Log(
			"caller", log.DefaultCaller,
			"msg", "restart function",
		)

		return runner.Restart()
	}

	return &actorQuerier{
			Actor: actor.Actor{
				// and the methods for starting and stopping the extension
				Execute: func() error {

					// Start the osqueryd instance
					if err := runner.Start(); err != nil {
						return fmt.Errorf("launching osquery instance: %w", err)
					}

					// If we're using osquery transport, we don't need the extension
					if opts.Transport == "osquery" {
						level.Debug(logger).Log("msg", "Using osquery transport, skipping extension startup")

						// TODO: remove when underlying libs are refactored
						// everything exits right now, so block this actor on the context finishing
						<-ctx.Done()
						return nil
					}

					// The runner allows querying the osqueryd instance from the extension.
					// Used by the Enroll method below to get initial enrollment details.
					ext.SetQuerier(runner)

					// It's not clear to me _why_ the extension
					// ever called Enroll here. From what
					// I can tell, this would cause the
					// launcher extension to do a bunch of
					// initial setup (create node key,
					// etc). But, this is also done by
					// osquery. And having them both
					// attempt it is a bit racey.  I'm not
					// so confident to outright remove it,
					// so it's gated behind this debugging
					// environment.
					if os.Getenv("LAUNCHER_DEBUG_ENROLL_FIRST") == "true" {
						// enroll this launcher with the server
						_, invalid, err := ext.Enroll(ctx)
						if err != nil {
							return fmt.Errorf("enrolling host: %w", err)
						}
						if invalid {
							return fmt.Errorf("invalid enroll secret: %w", err)
						}
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
			querier: runner.Query,
		},
		restartFunc,
		runner.Shutdown,
		nil
}

// commonRunnerOptions returns osquery runtime options common to all transports
func commonRunnerOptions(logger log.Logger, ktx *types.Knapsack, opts *launcher.Options) []runtime.OsqueryInstanceOption {
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
		runtime.WithOsqueryExtensionPlugins(ktable.LauncherTables(ktx, opts)...),
		runtime.WithStdout(osqueryStdoutLogger),
		runtime.WithStderr(osqueryStderrLogger),
		runtime.WithLogger(logger),
		runtime.WithOsqueryVerbose(opts.OsqueryVerbose),
		runtime.WithOsqueryFlags(opts.OsqueryFlags),
		runtime.WithAugeasLensFunction(augeas.InstallLenses),
		runtime.WithAutoloadedExtensions(opts.AutoloadedExtensions...),
	}
}

// osqueryRunnerOptions returns the osquery runtime options when using native osquery transport
func osqueryRunnerOptions(logger log.Logger, ktx *types.Knapsack, opts *launcher.Options) ([]runtime.OsqueryInstanceOption, error) {
	// As osquery requires TLS server certs, we'll  use our embedded defaults if not specified
	caCertFile := opts.RootPEM
	if caCertFile == "" {
		var err error
		caCertFile, err = internal.InstallCaCerts(opts.RootDirectory)
		if err != nil {
			return nil, fmt.Errorf("writing CA certs: %w", err)
		}
	}

	runtimeOptions := append(
		commonRunnerOptions(logger, ktx, opts),
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
func grpcRunnerOptions(logger log.Logger, ktx *types.Knapsack, opts *launcher.Options, ext *osquery.Extension) []runtime.OsqueryInstanceOption {
	return append(
		commonRunnerOptions(logger, ktx, opts),
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

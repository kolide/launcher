package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/ee/agent/types"
	kolidelog "github.com/kolide/launcher/ee/log/osquerylogs"
	"github.com/kolide/launcher/pkg/augeas"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
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
func createExtensionRuntime(ctx context.Context, k types.Knapsack, launcherClient service.KolideService) (
	run *actorQuerier,
	restart func() error, // restart osqueryd runner
	shutdown func() error, // shutdown osqueryd runner
	err error,
) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

	// read the enroll secret, if either it or the path has been specified
	var enrollSecret string
	if k.EnrollSecret() != "" {
		enrollSecret = k.EnrollSecret()
	} else if k.EnrollSecretPath() != "" {
		content, err := os.ReadFile(k.EnrollSecretPath())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("could not read enroll_secret_path: %s: %w", k.EnrollSecretPath(), err)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	}

	// create the osquery extension
	extOpts := osquery.ExtensionOpts{
		EnrollSecret:                      enrollSecret,
		Logger:                            logger,
		LoggingInterval:                   k.LoggingInterval(),
		RunDifferentialQueriesImmediately: k.EnableInitialRunner(),
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
	if k.LogMaxBytesPerBatch() != 0 {
		if k.Transport() == "grpc" && k.LogMaxBytesPerBatch() > 3 {
			level.Info(logger).Log(
				"msg", "LogMaxBytesPerBatch is set above the grpc recommended maximum of 3. Expect errors",
				"LogMaxBytesPerBatch", k.LogMaxBytesPerBatch(),
			)
		}
		extOpts.MaxBytesPerBatch = k.LogMaxBytesPerBatch() << 20
	} else if k.Transport() == "grpc" {
		extOpts.MaxBytesPerBatch = 3 << 20
	} else if k.Transport() != "grpc" {
		extOpts.MaxBytesPerBatch = 5 << 20
	}

	// create the extension
	ext, err := osquery.NewExtension(launcherClient, k, extOpts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("starting grpc extension: %w", err)
	}

	var runnerOptions []runtime.OsqueryInstanceOption

	if k.Transport() == "osquery" {
		var err error
		runnerOptions, err = osqueryRunnerOptions(logger, k)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("creating osquery runner options: %w", err)
		}
	} else {
		runnerOptions = grpcRunnerOptions(logger, k, ext)
	}

	runner := runtime.LaunchUnstartedInstance(runnerOptions...)

	restartFunc := func() error {
		level.Debug(logger).Log(
			"caller", log.DefaultCaller,
			"msg", "restart function",
		)

		return runner.Restart()
	}

	osqCtx, osqCancel := context.WithCancel(ctx)

	return &actorQuerier{
			Actor: actor.Actor{
				// and the methods for starting and stopping the extension
				Execute: func() error {
					// Attempt to enroll before starting up osquery. If we can't enroll now, don't error out --
					// we'll attempt again the first time osquery calls launcher plugins.
					_, invalid, err := ext.Enroll(osqCtx)
					if err != nil {
						level.Debug(logger).Log("msg", "error performing enrollment", "err", err)
					} else if invalid {
						level.Debug(logger).Log("msg", "invalid enroll secret", "err", err)
					}

					// Start the osqueryd instance -- pass in cancel so the osquery runner can let
					// this function know to stop waiting when the runner shuts down
					if err := runner.Start(osqCancel); err != nil {
						return fmt.Errorf("launching osquery instance: %w", err)
					}

					// If we're using osquery transport, we don't need the extension
					if k.Transport() == "osquery" {
						level.Debug(logger).Log("msg", "Using osquery transport, skipping extension startup")

						// TODO: remove when underlying libs are refactored
						// everything exits right now, so block this actor on the context finishing
						<-osqCtx.Done()
						return nil
					}

					// The runner allows querying the osqueryd instance from the extension.
					ext.SetQuerier(runner)

					// start the extension
					ext.Start()

					level.Info(logger).Log("msg", "extension started")

					// TODO: remove when underlying libs are refactored
					// everything exits right now, so block this actor on the context finishing
					<-osqCtx.Done()
					return nil
				},
				Interrupt: func(_ error) {
					ext.Shutdown()
					if runner != nil {
						if err := runner.Shutdown(); err != nil {
							level.Info(logger).Log("msg", "error shutting down runtime", "err", err)
							level.Debug(logger).Log("msg", "error shutting down runtime", "err", err, "stack", fmt.Sprintf("%+v", err))
						}
					}
					osqCancel()
				},
			},
			querier: runner.Query,
		},
		restartFunc,
		runner.Shutdown,
		nil
}

// commonRunnerOptions returns osquery runtime options common to all transports
func commonRunnerOptions(logger log.Logger, k types.Knapsack) []runtime.OsqueryInstanceOption {
	// create the logging adapters for osquery
	osqueryStderrLogger := kolidelog.NewOsqueryLogAdapter(
		k.Slogger().With(
			"component", "osquery",
			"osqlevel", "stderr",
		),
		k.RootDirectory(),
		kolidelog.WithLevel(slog.LevelInfo),
	)
	osqueryStdoutLogger := kolidelog.NewOsqueryLogAdapter(
		k.Slogger().With(
			"component", "osquery",
			"osqlevel", "stdout",
		),
		k.RootDirectory(),
		kolidelog.WithLevel(slog.LevelDebug),
	)

	return []runtime.OsqueryInstanceOption{
		runtime.WithKnapsack(k),
		runtime.WithOsquerydBinary(k.OsquerydPath()),
		runtime.WithRootDirectory(k.RootDirectory()),
		runtime.WithOsqueryExtensionPlugins(ktable.LauncherTables(k)...),
		runtime.WithStdout(osqueryStdoutLogger),
		runtime.WithStderr(osqueryStderrLogger),
		runtime.WithLogger(logger),
		runtime.WithOsqueryVerbose(k.OsqueryVerbose()),
		runtime.WithOsqueryFlags(k.OsqueryFlags()),
		runtime.WithAugeasLensFunction(augeas.InstallLenses),
		runtime.WithAutoloadedExtensions(k.AutoloadedExtensions()...),
		runtime.WithUpdateDirectory(k.UpdateDirectory()),
		runtime.WithUpdateChannel(k.UpdateChannel()),
		runtime.WithEnableWatchdog(true),
	}
}

// osqueryRunnerOptions returns the osquery runtime options when using native osquery transport
func osqueryRunnerOptions(logger log.Logger, k types.Knapsack) ([]runtime.OsqueryInstanceOption, error) {
	// As osquery requires TLS server certs, we'll  use our embedded defaults if not specified
	caCertFile := k.RootPEM()
	if caCertFile == "" {
		var err error
		caCertFile, err = internal.InstallCaCerts(k.RootDirectory())
		if err != nil {
			return nil, fmt.Errorf("writing CA certs: %w", err)
		}
	}

	runtimeOptions := append(
		commonRunnerOptions(logger, k),
		runtime.WithConfigPluginFlag("tls"),
		runtime.WithDistributedPluginFlag("tls"),
		runtime.WithLoggerPluginFlag("tls"),
		runtime.WithTlsConfigEndpoint(k.OsqueryTlsConfigEndpoint()),
		runtime.WithTlsDistributedReadEndpoint(k.OsqueryTlsDistributedReadEndpoint()),
		runtime.WithTlsDistributedWriteEndpoint(k.OsqueryTlsDistributedWriteEndpoint()),
		runtime.WithTlsEnrollEndpoint(k.OsqueryTlsEnrollEndpoint()),
		runtime.WithTlsHostname(k.KolideServerURL()),
		runtime.WithTlsLoggerEndpoint(k.OsqueryTlsLoggerEndpoint()),
		runtime.WithTlsServerCerts(caCertFile),
	)

	// Enroll secrets... Either we pass a file, or we write a
	// secret, and pass _that_ file
	if k.EnrollSecretPath() != "" {
		runtimeOptions = append(runtimeOptions, runtime.WithEnrollSecretPath(k.EnrollSecretPath()))
	} else if k.EnrollSecret() != "" {
		filename := filepath.Join(k.RootDirectory(), "secret")
		os.WriteFile(filename, []byte(k.EnrollSecret()), 0400)
		runtimeOptions = append(runtimeOptions, runtime.WithEnrollSecretPath(filename))
	}

	return runtimeOptions, nil
}

// grpcRunnerOptions returns the osquery runtime options when using launcher transports. (Eg: grpc or jsonrpc)
func grpcRunnerOptions(logger log.Logger, k types.Knapsack, ext *osquery.Extension) []runtime.OsqueryInstanceOption {
	return append(
		commonRunnerOptions(logger, k),
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

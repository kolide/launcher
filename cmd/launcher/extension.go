package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/launcher"
	kolidelog "github.com/kolide/launcher/pkg/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	ktable "github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquerylogger "github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

// TODO: the extension, runtime, and client are all kind of entangled here. Untangle the underlying libraries and separate into units
func createExtensionRuntime(ctx context.Context, db *bolt.DB, launcherClient service.KolideService, opts *launcher.Options) (
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

	// create the extension
	ext, err := osquery.NewExtension(launcherClient, db, extOpts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "starting grpc extension")
	}

	// create the logging adapters for osquery
	osqueryStderrLogger := kolidelog.NewOsqueryLogAdapter(
		logger,
		kolidelog.WithLevelFunc(level.Info),
		kolidelog.WithKeyValue("component", "osquery"),
		kolidelog.WithKeyValue("level", "stderr"),
	)
	osqueryStdoutLogger := kolidelog.NewOsqueryLogAdapter(
		logger,
		kolidelog.WithLevelFunc(level.Debug),
		kolidelog.WithKeyValue("component", "osquery"),
		kolidelog.WithKeyValue("level", "stdout"),
	)

	runner := runtime.LaunchUnstartedInstance(
		runtime.WithOsquerydBinary(opts.OsquerydPath),
		runtime.WithRootDirectory(opts.RootDirectory),
		runtime.WithConfigPluginFlag("kolide_grpc"),
		runtime.WithLoggerPluginFlag("kolide_grpc"),
		runtime.WithDistributedPluginFlag("kolide_grpc"),
		runtime.WithOsqueryExtensionPlugins(
			config.NewPlugin("kolide_grpc", ext.GenerateConfigs),
			distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults),
			osquerylogger.NewPlugin("kolide_grpc", ext.LogString),
		),
		runtime.WithOsqueryExtensionPlugins(ktable.LauncherTables(db, opts)...),
		runtime.WithStdout(osqueryStdoutLogger),
		runtime.WithStderr(osqueryStderrLogger),
		runtime.WithLogger(logger),
		runtime.WithOsqueryVerbose(opts.OsqueryVerbose),
	)

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
				level.Info(logger).Log("msg", "extension interrupted", "err", err, "stack", fmt.Sprintf("%+v", err))
				ext.Shutdown()
				if runner != nil {
					if err := runner.Shutdown(); err != nil {
						level.Info(logger).Log("msg", "error shutting down runtime", "err", err, "stack", fmt.Sprintf("%+v", err))
					}
				}
			},
		},
		restartFunc,
		runner.Shutdown,
		nil
}

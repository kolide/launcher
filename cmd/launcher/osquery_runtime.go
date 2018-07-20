package main

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	kolidelog "github.com/kolide/launcher/pkg/log"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquerylogger "github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

func createOSQueryRuntime(ctx context.Context, rootDirectory string, db *bolt.DB, logger *kolidelog.Logger, opts *options) (*actor.Actor, error) {
	// create a log adapter for osquery
	osqueryLogger := &kolidelog.OsqueryLogAdapter{Logger: level.Debug(log.With(logger, "component", "osquery"))}
	// create the osquery runtime
	runner := runtime.New(
		runtime.WithOsquerydBinary(opts.osquerydPath),
		runtime.WithRootDirectory(rootDirectory),
		runtime.WithConfigPluginFlag("kolide_grpc"),
		runtime.WithLoggerPluginFlag("kolide_grpc"),
		runtime.WithDistributedPluginFlag("kolide_grpc"),
		runtime.WithOsqueryExtensionPlugin(config.NewPlugin("kolide_grpc", ext.GenerateConfigs)),
		runtime.WithOsqueryExtensionPlugin(osquerylogger.NewPlugin("kolide_grpc", ext.LogString)),
		runtime.WithOsqueryExtensionPlugin(distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)),
		runtime.WithOsqueryExtensionPlugin(table.LauncherIdentifierTable(db)),
		runtime.WithStdout(osqueryLogger),
		runtime.WithStderr(osqueryLogger),
		runtime.WithLogger(logger),
	)

	return &actor.Actor{
		Execute: func() error {
			err := runner.Start()
			if err != nil {
				return nil, errors.Wrap(err, "launching osquery instance")
			}
			return nil
		},
		Interrupt: func(err error) {

		},
	}, nil
}

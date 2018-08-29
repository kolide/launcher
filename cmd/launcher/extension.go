package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"io/ioutil"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	kolidelog "github.com/kolide/launcher/pkg/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquerylogger "github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

// TODO: the extension, runtime, and client are all kind of entangled here. Untangle the underlying libraries and separate into units
func createExtensionRuntime(ctx context.Context, rootDirectory string, db *bolt.DB, logger log.Logger, opts *options) (
	run *actor.Actor,
	restart func() error, // restart osqueryd runner
	shutdown func() error, // shutdown osqueryd runner
	err error,
) {
	// read the enroll secret, if either it or the path has been specified
	var enrollSecret string
	if opts.enrollSecret != "" {
		enrollSecret = opts.enrollSecret
	} else if opts.enrollSecretPath != "" {
		content, err := ioutil.ReadFile(opts.enrollSecretPath)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "could not read enroll_secret_path: %s", opts.enrollSecretPath)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	}

	// create the certificate pool
	var rootPool *x509.CertPool
	if opts.rootPEM != "" {
		rootPool = x509.NewCertPool()
		pemContents, err := ioutil.ReadFile(opts.rootPEM)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "reading root certs PEM at path: %s", opts.rootPEM)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			return nil, nil, nil, errors.Errorf("found no valid certs in PEM at path: %s", opts.rootPEM)
		}
	}

	// connect to the grpc server
	grpcConn, err := service.DialGRPC(opts.kolideServerURL, opts.insecureTLS, opts.insecureGRPC, opts.certPins, rootPool, logger)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "dialing grpc server")
	}

	// create the client of the grpc service
	launcherClient := service.New(grpcConn, level.Debug(logger))

	// create the osquery extension
	extOpts := osquery.ExtensionOpts{
		EnrollSecret:    enrollSecret,
		Logger:          logger,
		LoggingInterval: opts.loggingInterval,
	}

	// create the extension
	ext, err := osquery.NewExtension(launcherClient, db, extOpts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "starting grpc extension")
	}

	// create the logging adapter for osquery
	osqueryLogger := &kolidelog.OsqueryLogAdapter{Logger: level.Debug(log.With(logger, "component", "osquery"))}

	runner := runtime.LaunchUnstartedInstance(
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
				level.Info(logger).Log("msg", "extension interrupted", "err", err)
				grpcConn.Close()
				ext.Shutdown()
				if runner != nil {
					if err := runner.Shutdown(); err != nil {
						level.Info(logger).Log("msg", "error shutting down runtime", "err", err)
					}
				}
			},
		},
		runner.Restart,
		runner.Shutdown,
		nil
}

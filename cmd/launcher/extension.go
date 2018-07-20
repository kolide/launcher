package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"io/ioutil"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	kolidelog "github.com/kolide/launcher/pkg/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/service"
	"github.com/pkg/errors"
)

// a client is the runtime, and also the methods for starting/interrupting it
type client struct {
	runner *runtime.Runner
	*actor.Actor
}

// TODO: the extension, runtime, and client are all kind of entangled here. Untangle the underlying libraries and separate into units
func createExtension(ctx context.Context, db *bolt.DB, logger *kolidelog.Logger, opts *options) (*client, error) {
	// read the enroll secret, if either it or the path has been specified
	var enrollSecret string
	if opts.enrollSecret != "" {
		enrollSecret = opts.enrollSecret
	} else if opts.enrollSecretPath != "" {
		content, err := ioutil.ReadFile(opts.enrollSecretPath)
		if err != nil {
			return nil, errors.Wrapf(err, "could not read enroll_secret_path: %s", opts.enrollSecretPath)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	}

	// create the certificate pool
	var rootPool *x509.CertPool
	if opts.rootPEM != "" {
		rootPool = x509.NewCertPool()
		pemContents, err := ioutil.ReadFile(opts.rootPEM)
		if err != nil {
			return nil, errors.Wrapf(err, "reading root certs PEM at path: %s", opts.rootPEM)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			return nil, errors.Errorf("found no valid certs in PEM at path: %s", opts.rootPEM)
		}
	}

	// connect to the grpc server
	grpcConn, err := service.DialGRPC(opts.kolideServerURL, opts.insecureTLS, opts.insecureGRPC, opts.certPins, rootPool, logger)
	if err != nil {
		return nil, errors.Wrap(err, "dialing grpc server")
	}

	// create the client of the grpc service
	launcherClient := service.NewClient(grpcConn, level.Debug(logger))

	// create the osquery extension
	extOpts := osquery.ExtensionOpts{
		EnrollSecret:    enrollSecret,
		Logger:          logger,
		LoggingInterval: opts.loggingInterval,
	}
	ext, err := osquery.NewExtension(launcherClient, db, extOpts)
	if err != nil {
		return nil, errors.Wrap(err, "starting grpc extension")
	}

	return &client{
		// add the runner to the client
		runner,
		// and the methods for starting and stopping it
		&actor.Actor{
			Execute: func() error {
				println("\nclient started\n")
				// Start the osqueryd instance

				// The runner allows querying the osqueryd instance from the extension.
				// Used by the Enroll method below to get initial enrollment details.
				ext.SetQuerier(runner)

				// Start the extension
				ext.Start()

				// enroll the host with the extension
				_, invalid, err := ext.Enroll(ctx)
				if err != nil {
					logger.Fatal("err", errors.Wrap(err, "enrolling host"))
				}
				if invalid {
					logger.Fatal(errors.Wrap(err, "invalid enroll secret"))
				}

				// TODO: remove when underlying libs are refactors
				// everything exits right now, so block this actor on the context finishing
				<-ctx.Done()
				return nil
			},
			Interrupt: func(err error) {
				println("\nclient interrupted\n")
				grpcConn.Close()
				ext.Shutdown()
				return
			},
		},
	}, nil
}

func (c *client) Shutdown() error {
	if c.runner != nil {
		return c.runner.Shutdown()
	}
	return nil
}

func (c *client) Restart() error {
	if c.runner != nil {
		return c.runner.Restart()
	}
	return nil
}

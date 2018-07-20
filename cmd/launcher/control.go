package main

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/control"
	kolideLog "github.com/kolide/launcher/pkg/log"
	"github.com/pkg/errors"
)

func createControl(ctx context.Context, db *bolt.DB, logger *kolideLog.Logger, opts *options) (*actor.Actor, error) {
	level.Debug(logger).Log("msg", "creating control client")

	controlOpts := []control.Option{
		control.WithLogger(logger),
		control.WithGetShellsInterval(opts.getShellsInterval),
	}
	if opts.insecureTLS {
		controlOpts = append(controlOpts, control.WithInsecureSkipVerify())
	}
	if opts.disableControlTLS {
		controlOpts = append(controlOpts, control.WithDisableTLS())
	}
	controlClient, err := control.NewClient(db, opts.controlServerURL, controlOpts...)
	if err != nil {
		logger.Fatal(errors.Wrap(err, "creating control client"))
	}

	return &actor.Actor{
		Execute: func() error {
			println("\ncontrol started\n")
			controlClient.Start(ctx)
			return nil
		},
		Interrupt: func(err error) {
			println("\ncontrol interrupted\n")
			controlClient.Stop()
		},
	}, nil
}

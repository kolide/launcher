package main

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/control"
	"github.com/pkg/errors"
)

func createControl(ctx context.Context, db *bolt.DB, logger log.Logger, opts *options) (*actor.Actor, error) {
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
	controlClient, err := control.NewControlClient(db, opts.controlServerURL, controlOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "creating control client")
	}

	return &actor.Actor{
		Execute: func() error {
			level.Info(logger).Log("msg", "control started")
			controlClient.Start(ctx)
			return nil
		},
		Interrupt: func(err error) {
			if err != nil {
				level.Info(logger).Log("err", err)
			}
			level.Info(logger).Log("msg", "control interrupted")
			controlClient.Stop()
		},
	}, nil
}

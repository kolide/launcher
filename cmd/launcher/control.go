//go:build !windows
// +build !windows

package main

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/control"
	"github.com/kolide/launcher/pkg/launcher"

	"go.etcd.io/bbolt"
)

func createControl(ctx context.Context, db *bbolt.DB, logger log.Logger, opts *launcher.Options) (*actor.Actor, error) {
	level.Debug(logger).Log("msg", "creating control client")

	controlOpts := []control.Option{
		control.WithLogger(logger),
	}
	if opts.InsecureTLS {
		controlOpts = append(controlOpts, control.WithInsecureSkipVerify())
	}
	if opts.DisableControlTLS {
		controlOpts = append(controlOpts, control.WithDisableTLS())
	}
	controlClient, err := control.NewControlClient(db, opts.ControlServerURL, controlOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating control client: %w", err)
	}

	return &actor.Actor{
		Execute: func() error {
			level.Info(logger).Log("msg", "control started")
			controlClient.Start(ctx)
			return nil
		},
		Interrupt: func(err error) {
			level.Info(logger).Log("msg", "control interrupted", "err", err)
			controlClient.Stop()
		},
	}, nil
}

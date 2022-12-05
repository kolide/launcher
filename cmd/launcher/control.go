package main

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/launcher"
)

func createHTTPClient(ctx context.Context, logger log.Logger, opts *launcher.Options) (*control.HTTPClient, error) {
	level.Debug(logger).Log("msg", "creating control http client")

	clientOpts := []control.HTTPClientOption{}
	if opts.InsecureTLS {
		clientOpts = append(clientOpts, control.WithInsecureSkipVerify())
	}
	if opts.DisableControlTLS {
		clientOpts = append(clientOpts, control.WithDisableTLS())
	}
	client, err := control.NewControlHTTPClient(opts.ControlServerURL, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating control http client: %w", err)
	}

	return client, nil
}

func createControlService(ctx context.Context, logger log.Logger, opts *launcher.Options) (*actor.Actor, error) {
	level.Debug(logger).Log("msg", "creating control service")

	client, err := createHTTPClient(ctx, logger, opts)
	if err != nil {
		return nil, err
	}

	controlOpts := []control.Option{
		control.WithRequestInterval(opts.ControlRequestInterval),
	}
	service := control.NewControlService(logger, client, controlOpts...)

	return &actor.Actor{
		Execute: func() error {
			level.Info(logger).Log("msg", "control service started")
			service.Start(ctx)
			return nil
		},
		Interrupt: func(err error) {
			level.Info(logger).Log("msg", "control service interrupted", "err", err)
			service.Stop()
		},
	}, nil
}

package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/launcher"
)

func createHTTPClient(ctx context.Context, logger log.Logger, opts *launcher.Options) (*control.HTTPClient, error) {
	level.Debug(logger).Log("msg", "creating control http client")

	clientOpts := []control.HTTPClientOption{}
	if opts.InsecureControlTLS {
		clientOpts = append(clientOpts, control.WithInsecureSkipVerify())
	}
	if opts.DisableControlTLS {
		clientOpts = append(clientOpts, control.WithDisableTLS())
	}
	client, err := control.NewControlHTTPClient(logger, opts.ControlServerURL, http.DefaultClient, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating control http client: %w", err)
	}

	return client, nil
}

func createControlService(ctx context.Context, logger log.Logger, store types.GetterSetter, opts *launcher.Options) (*control.ControlService, error) {
	level.Debug(logger).Log("msg", "creating control service")

	client, err := createHTTPClient(ctx, logger, opts)
	if err != nil {
		return nil, err
	}

	controlOpts := []control.Option{
		control.WithRequestInterval(opts.ControlRequestInterval),
		control.WithStore(store),
	}
	service := control.New(logger, client, controlOpts...)

	return service, nil
}

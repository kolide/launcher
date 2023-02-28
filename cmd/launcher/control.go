package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/launcher"
	"go.etcd.io/bbolt"
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

func createControlService(ctx context.Context, logger log.Logger, db *bbolt.DB, opts *launcher.Options) (*control.ControlService, error) {
	level.Debug(logger).Log("msg", "creating control service")

	client, err := createHTTPClient(ctx, logger, opts)
	if err != nil {
		return nil, err
	}

	getset := storage.NewBBoltKeyValueStore(logger, db, "control_service_data")

	controlOpts := []control.Option{
		control.WithRequestInterval(opts.ControlRequestInterval),
		control.WithGetterSetter(getset),
	}
	service := control.New(logger, ctx, client, controlOpts...)

	return service, nil
}

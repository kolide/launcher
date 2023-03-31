package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/agent/types"
)

func createHTTPClient(ctx context.Context, logger log.Logger, k types.Knapsack) (*control.HTTPClient, error) {
	level.Debug(logger).Log("msg", "creating control http client")

	clientOpts := []control.HTTPClientOption{}
	if k.InsecureControlTLS() {
		clientOpts = append(clientOpts, control.WithInsecureSkipVerify())
	}
	if k.DisableControlTLS() {
		clientOpts = append(clientOpts, control.WithDisableTLS())
	}
	client, err := control.NewControlHTTPClient(logger, k.ControlServerURL(), http.DefaultClient, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating control http client: %w", err)
	}

	return client, nil
}

func createControlService(ctx context.Context, logger log.Logger, store types.GetterSetter, k types.Knapsack) (*control.ControlService, error) {
	level.Debug(logger).Log("msg", "creating control service")

	client, err := createHTTPClient(ctx, logger, k)
	if err != nil {
		return nil, err
	}

	controlOpts := []control.Option{
		control.WithStore(k.ControlStore()),
	}
	service := control.New(logger, k, client, controlOpts...)

	return service, nil
}

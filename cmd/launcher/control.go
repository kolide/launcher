package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/traces"
)

func createHTTPClient(ctx context.Context, k types.Knapsack) (*control.HTTPClient, error) {
	k.Slogger().Log(ctx, slog.LevelDebug,
		"creating control http client",
	)

	clientOpts := []control.HTTPClientOption{}
	if k.InsecureControlTLS() {
		clientOpts = append(clientOpts, control.WithInsecureSkipVerify())
	}
	if k.DisableControlTLS() {
		clientOpts = append(clientOpts, control.WithDisableTLS())
	}

	logger := k.Slogger().With("component", "control_http_client")
	client, err := control.NewControlHTTPClient(k.ControlServerURL(), http.DefaultClient, logger, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating control http client: %w", err)
	}

	return client, nil
}

func createControlService(ctx context.Context, k types.Knapsack) (*control.ControlService, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	k.Slogger().Log(ctx, slog.LevelDebug,
		"creating control service",
	)

	client, err := createHTTPClient(ctx, k)
	if err != nil {
		return nil, err
	}

	controlOpts := []control.Option{
		control.WithStore(k.ControlStore()),
	}
	service := control.New(k, client, controlOpts...)

	return service, nil
}

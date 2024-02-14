package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/traces"
)

func createExtensionRuntime(ctx context.Context, k types.Knapsack, launcherClient service.KolideService) (*osquery.Extension, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)
	slogger := k.Slogger().With("component", "osquery_extension_actor")

	// read the enroll secret, if either it or the path has been specified
	var enrollSecret string
	if k.EnrollSecret() != "" {
		enrollSecret = k.EnrollSecret()
	} else if k.EnrollSecretPath() != "" {
		content, err := os.ReadFile(k.EnrollSecretPath())
		if err != nil {
			return nil, fmt.Errorf("could not read enroll_secret_path: %s: %w", k.EnrollSecretPath(), err)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	}

	// create the osquery extension
	extOpts := osquery.ExtensionOpts{
		EnrollSecret:                      enrollSecret,
		Logger:                            logger, // Preserved only for temporary use in agent.SetupKeys
		LoggingInterval:                   k.LoggingInterval(),
		RunDifferentialQueriesImmediately: k.EnableInitialRunner(),
	}

	// Setting MaxBytesPerBatch is a tradeoff. If it's too low, we
	// can never send a large result. But if it's too high, we may
	// not be able to send the data over a low bandwidth
	// connection before the connection is timed out.
	//
	// The logic for setting this is spread out. The underlying
	// extension defaults to 3mb, to support GRPC's hardcoded 4MB
	// limit. But as we're transport aware here. we can set it to
	// 5MB for others.
	if k.LogMaxBytesPerBatch() != 0 {
		if k.Transport() == "grpc" && k.LogMaxBytesPerBatch() > 3 {
			slogger.Log(ctx, slog.LevelInfo,
				"LogMaxBytesPerBatch is set above the grpc recommended maximum of 3. Expect errors",
				"log_max_bytes_per_batch", k.LogMaxBytesPerBatch(),
			)
		}
		extOpts.MaxBytesPerBatch = k.LogMaxBytesPerBatch() << 20
	} else if k.Transport() == "grpc" {
		extOpts.MaxBytesPerBatch = 3 << 20
	} else if k.Transport() != "grpc" {
		extOpts.MaxBytesPerBatch = 5 << 20
	}

	// create the extension
	return osquery.NewExtension(ctx, launcherClient, k, extOpts)
}

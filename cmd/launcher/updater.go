package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/kit/actor"
	"github.com/kolide/updater/tuf"
	"github.com/pkg/errors"
)

type updaterConfig struct {
	Logger             log.Logger
	RootDirectory      string
	AutoupdateInterval time.Duration
	UpdateChannel      autoupdate.UpdateChannel

	NotaryURL string
	MirrorURL string
	GUNPrefix string

	HTTPClient *http.Client

	SigChannel chan os.Signal
}

func createUpdater(
	ctx context.Context,
	binaryPath string,
	finalizer autoupdate.UpdateFinalizer,
	logger log.Logger,
	config *updaterConfig,
) (*actor.Actor, error) {
	// create the updater
	updater, err := autoupdate.NewUpdater(
		binaryPath,
		config.RootDirectory,
		config.GUNPrefix,
		config.Logger,
		autoupdate.WithLogger(config.Logger),
		autoupdate.WithHTTPClient(config.HTTPClient),
		autoupdate.WithNotaryURL(config.NotaryURL),
		autoupdate.WithMirrorURL(config.MirrorURL),
		autoupdate.WithFinalizer(finalizer),
		autoupdate.WithUpdateChannel(config.UpdateChannel),
		autoupdate.WithSigChannel(config.SigChannel),
	)
	if err != nil {
		return nil, err
	}

	// create a variable to hold the stop function that the updater returns,
	// so that we can pass it into the interrupt function
	var stop func()

	return &actor.Actor{
		Execute: func() error {
			level.Info(logger).Log("msg", "updater started")

			// run the updater and set the stop function so that the interrupt has access to it
			stop, err = updater.Run(tuf.WithFrequency(config.AutoupdateInterval), tuf.WithLogger(logger))
			if err != nil {
				return errors.Wrap(err, "running updater")
			}

			// TODO: remove when underlying libs are refactored
			// everything exits right now, so block this actor on the context finishing
			<-ctx.Done()

			return nil
		},
		Interrupt: func(err error) {
			level.Info(logger).Log("msg", "updater interrupted", "err", err, "stack", fmt.Sprintf("%+v", err))
			if stop != nil {
				stop()
			}
		},
	}, nil
}

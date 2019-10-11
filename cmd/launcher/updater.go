package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/updater/tuf"
)

type updaterConfig struct {
	Logger             log.Logger
	RootDirectory      string
	AutoupdateInterval time.Duration
	UpdateChannel      autoupdate.UpdateChannel

	NotaryURL    string
	MirrorURL    string
	NotaryPrefix string

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
		config.Logger,
		autoupdate.WithLogger(config.Logger),
		autoupdate.WithHTTPClient(config.HTTPClient),
		autoupdate.WithNotaryURL(config.NotaryURL),
		autoupdate.WithMirrorURL(config.MirrorURL),
		autoupdate.WithNotaryPrefix(config.NotaryPrefix),
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
	stopChan := make(chan bool)

	return &actor.Actor{
		Execute: func() error {
			// Failing to start the updater is not a fatal launcher
			// error. If there's a problem, sleep and try
			// again. Implementing this is a bit gnarly. In the event of a
			// success, we get a nil error, and a stop function. But I don't
			// see a simple way to ensure the updater is still running in
			// the background.
			for {
				level.Info(logger).Log("msg", "updater started")

				// run the updater and set the stop function so that the interrupt has access to it
				stop, err = updater.Run(tuf.WithFrequency(config.AutoupdateInterval), tuf.WithLogger(logger))
				if err == nil {
					break
				}

				// err != nil, log it and loop again
				level.Error(logger).Log("msg", "Error running updater. Will retry", "err", err)
				select {
				case <-stopChan:
					level.Debug(logger).Log("msg", "stop requested. Breaking loop")
					return nil
				case <-time.After(30 * time.Minute):
					break
				}

			}

			// Don't exit unless there's a done signal TODO: remove when
			// underlying libs are refactored, everything exits right now,
			// so block this actor on the context finishing
			level.Debug(logger).Log("msg", "waiting")
			<-ctx.Done()

			return nil
		},
		Interrupt: func(err error) {
			level.Info(logger).Log("msg", "updater interrupted", "err", err, "stack", fmt.Sprintf("%+v", err))

			// non-blocking channel send
			select {
			case stopChan <- true:
			default:
				level.Debug(logger).Log("msg", "failed to send stop signal")
			}

			if stop != nil {
				stop()
			}
		},
	}, nil
}

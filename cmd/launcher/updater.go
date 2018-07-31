package main

import (
	"context"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/autoupdate"
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

	HTTPClient *http.Client
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
		autoupdate.WithFinalizer(finalizer),
		autoupdate.WithUpdateChannel(config.UpdateChannel),
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

			// run the updater and set the stop function so that the interrupt has aaccess to it
			stop, err = updater.Run(tuf.WithFrequency(config.AutoupdateInterval))
			if err != nil {
				return errors.Wrap(err, "running updater")
			}

			// TODO: remove when underlying libs are refactored
			// everything exits right now, so block this actor on the context finishing
			<-ctx.Done()

			return nil
		},
		Interrupt: func(err error) {
			if err != nil {
				level.Info(logger).Log("err", err)
			}
			level.Info(logger).Log("msg", "updater interrupted")
			if stop != nil {
				stop()
			}
		},
	}, nil
}

func launcherFinalizer(logger log.Logger, shutdownOsquery func() error) func() error {
	return func() error {
		if err := shutdownOsquery(); err != nil {
			level.Info(logger).Log(
				"method", "launcherFinalizer",
				"err", err,
			)
		}
		// replace launcher
		if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
			return errors.Wrap(err, "restarting launcher")
		}
		return nil
	}
}

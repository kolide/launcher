package main

import (
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

func createUpdater(binaryPath string, finalizer autoupdate.UpdateFinalizer, config *updaterConfig) (*actor.Actor, error) {
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
			println("\nupdater started\n")
			stop, err = updater.Run(tuf.WithFrequency(config.AutoupdateInterval))
			return err
		},
		Interrupt: func(err error) {
			println("\nupdater interrupted\n")
			stop()
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

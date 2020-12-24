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

// updaterConfig is a struct of update related options. It's used to
// simplify the call to `createUpdater` from launcher's main blocks.
type updaterConfig struct {
	Logger             log.Logger
	RootDirectory      string // launcher's root dir. use for holding tuf staging and updates
	AutoupdateInterval time.Duration
	UpdateChannel      autoupdate.UpdateChannel
	InitialDelay       time.Duration // start delay, to avoid whomping critical early data

	NotaryURL    string
	MirrorURL    string
	NotaryPrefix string

	HTTPClient *http.Client

	SigChannel chan os.Signal
}

// createUpdater returns an Actor suitable for an oklog/run group. It
// is a light wrapper around autoupdate.NewUpdater to simplify having
// multiple ones in launcher.
func createUpdater(
	ctx context.Context,
	binaryPath string,
	finalizer autoupdate.UpdateFinalizer,
	config *updaterConfig,
) (*actor.Actor, error) {
	// create the updater
	updater, err := autoupdate.NewUpdater(
		binaryPath,
		config.RootDirectory,
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
			// When launcher first starts, we'd like the
			// Saas side to start recieving data
			// immediately. But, if updater is trying to
			// run, this creates an awkward pause for restart.
			// So, delay starting updates by an hour or two.
			if config.InitialDelay != 0 {
				time.Sleep(config.InitialDelay)
			}

			// Failing to start the updater is not a fatal launcher
			// error. If there's a problem, sleep and try
			// again. Implementing this is a bit gnarly. In the event of a
			// success, we get a nil error, and a stop function. But I don't
			// see a simple way to ensure the updater is still running in
			// the background.
			for {
				level.Info(config.Logger).Log("msg", "updater started")

				// run the updater and set the stop function so that the interrupt has access to it
				stop, err = updater.Run(tuf.WithFrequency(config.AutoupdateInterval), tuf.WithLogger(config.Logger))
				if err == nil {
					break
				}

				// err != nil, log it and loop again
				level.Error(config.Logger).Log("msg", "Error running updater. Will retry", "err", err)
				select {
				case <-stopChan:
					level.Debug(config.Logger).Log("msg", "stop requested. Breaking loop")
					return nil
				case <-time.After(30 * time.Minute):
					break
				}
			}

			// Don't exit unless there's a done signal TODO: remove when
			// underlying libs are refactored, everything exits right now,
			// so block this actor on the context finishing
			level.Debug(config.Logger).Log("msg", "waiting")
			<-ctx.Done()

			return nil
		},
		Interrupt: func(err error) {
			level.Info(config.Logger).Log("msg", "updater interrupted", "err", err)
			level.Debug(config.Logger).Log("msg", "updater interrupted", "err", err, "stack", fmt.Sprintf("%+v", err))

			// non-blocking channel send
			select {
			case stopChan <- true:
			default:
				level.Debug(config.Logger).Log("msg", "failed to send stop signal")
			}

			if stop != nil {
				stop()
			}
		},
	}, nil
}

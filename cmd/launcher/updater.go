package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
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

	binaryName := path.Base(binaryPath)

	return &actor.Actor{
		Execute: func() error {
			// Failing to start the updater is not a fatal launcher
			// error. If there's a problem, sleep and try
			// again. Implementing this is a bit gnarly. In the event of a
			// success, we get a nil error, and a stop function. But I don't
			// see a simple way to ensure the updater is still running in
			// the background.
			for {
				// When launcher first starts, we'd like the
				// server to start receiving data
				// immediately. But, if updater is trying to
				// run, this creates an awkward pause for restart.
				// So, delay starting updates by an hour or two.
				level.Debug(config.Logger).Log("msg", fmt.Sprintf("%s updater entering initial delay of %s", binaryName, config.InitialDelay))

				select {
				case <-stopChan:
					level.Debug(config.Logger).Log("msg", fmt.Sprintf("%s updater stopped requested during initial delay, Breaking loop", binaryName))
					return nil
				case <-time.After(config.InitialDelay):
					level.Debug(config.Logger).Log("msg", fmt.Sprintf("%s updater initial delay complete", binaryName))
					break
				}

				level.Info(config.Logger).Log("msg", fmt.Sprintf("%s updater starting", binaryName))

				// run the updater and set the stop function so that the interrupt has access to it
				stop, err = updater.Run(tuf.WithFrequency(config.AutoupdateInterval), tuf.WithLogger(config.Logger))
				if err == nil {
					break
				}

				// err != nil, log it and loop again
				level.Error(config.Logger).Log("msg", fmt.Sprintf("error running %s updater", binaryName), "err", err)
				select {
				case <-stopChan:
					level.Debug(config.Logger).Log("msg", fmt.Sprintf("%s updater stop requested, Breaking loop", binaryName))
					return nil
				case <-time.After(30 * time.Minute):
					break
				}
			}

			// Don't exit unless there's a done signal TODO: remove when
			// underlying libs are refactored, everything exits right now,
			// so block this actor on the context finishing
			level.Debug(config.Logger).Log("msg", fmt.Sprintf("%s updater waiting ... just sitting until done signal (see TODO)", binaryName))
			<-ctx.Done()

			return nil
		},

		Interrupt: func(err error) {
			level.Info(config.Logger).Log("msg", fmt.Sprintf("%s updater interrupted", binaryName), "err", err)
			level.Debug(config.Logger).Log("msg", fmt.Sprintf("%s updater interrupted", binaryName), "err", err, "stack", fmt.Sprintf("%+v", err))

			// non-blocking channel send
			select {
			case stopChan <- true:
			default:
				level.Debug(config.Logger).Log("msg", fmt.Sprintf("%s updater failed to send stop signal", binaryName))
			}

			if stop != nil {
				stop()
			}
		},
	}, nil
}

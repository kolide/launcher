package updater

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/updater/tuf"
)

// UpdaterConfig is a struct of update related options. It's used to
// simplify the call to `createUpdater` from launcher's main blocks.
type UpdaterConfig struct {
	Logger             log.Logger
	RootDirectory      string // launcher's root dir. use for holding tuf staging and updates
	AutoupdateInterval time.Duration
	UpdateChannel      autoupdate.UpdateChannel
	InitialDelay       time.Duration // start delay, to avoid whomping critical early data
	NotaryURL          string
	TufServerURL       string
	MirrorURL          string
	NotaryPrefix       string
	HTTPClient         *http.Client
	SigChannel         chan os.Signal
}

// NewUpdater returns an Actor suitable for an oklog/run group. It
// is a light wrapper around autoupdate.NewUpdater to simplify having
// multiple ones in launcher.
func NewUpdater(
	ctx context.Context,
	binaryPath string,
	finalizer autoupdate.UpdateFinalizer,
	config *UpdaterConfig,
) (*actor.Actor, error) {

	if config.Logger == nil {
		config.Logger = log.NewNopLogger()
	}

	config.Logger = log.With(config.Logger, "updater", filepath.Base(binaryPath))

	// create the updater
	updater, err := autoupdate.NewTufAutoupdater(
		config.TufServerURL,
		config.MirrorURL,
		binaryPath,
		config.RootDirectory,
		config.SigChannel,
		autoupdate.WithTufLogger(config.Logger),
		autoupdate.WithChannel(config.UpdateChannel),
		autoupdate.WithUpdateCheckInterval(config.AutoupdateInterval),
	)
	if err != nil {
		return nil, err
	}

	// create the fallback updater
	fallbackUpdater, err := autoupdate.NewUpdater(
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

	updateCmd := &updaterCmd{
		updater:                 updater,
		fallbackUpdater:         fallbackUpdater,
		ctx:                     ctx,
		stopChan:                make(chan bool),
		config:                  config,
		runUpdaterRetryInterval: 30 * time.Minute,
		monitorInterval:         1 * time.Hour,
	}

	return &actor.Actor{
		Execute:   updateCmd.execute,
		Interrupt: updateCmd.interrupt,
	}, nil
}

// updater allows us to mock *autoupdate.Updater during testing
type updater interface {
	Run(opts ...tuf.Option) (stop func(), err error)
	ErrorCount() int
}

type updaterCmd struct {
	updater                 updater
	fallbackUpdater         updater
	runFallback             bool
	ctx                     context.Context
	stopChan                chan bool
	stopExecution           func()
	config                  *UpdaterConfig
	runUpdaterRetryInterval time.Duration
	monitorInterval         time.Duration
}

func (u *updaterCmd) execute() error {
	// When launcher first starts, we'd like the
	// server to start receiving data
	// immediately. But, if updater is trying to
	// run, this creates an awkward pause for restart.
	// So, delay starting updates by an hour or two.
	level.Debug(u.config.Logger).Log("msg", "updater entering initial delay", "delay", u.config.InitialDelay)

	select {
	case <-u.stopChan:
		level.Debug(u.config.Logger).Log("msg", "updater stopped requested during initial delay, breaking loop")
		return nil
	case <-time.After(u.config.InitialDelay):
		level.Debug(u.config.Logger).Log("msg", "updater initial delay complete")
		break
	}

	timedOutErr := u.startUpdater()
	if timedOutErr != nil {
		<-u.ctx.Done()
		return nil
	}

	go u.monitorUpdater()

	level.Debug(u.config.Logger).Log("msg", "updater waiting ... just sitting until done signal")
	<-u.ctx.Done()

	return nil
}

func (u *updaterCmd) monitorUpdater() {
	// monitor new updater, falling back to old one
	errorThreshold := 5
	for {
		level.Debug(u.config.Logger).Log("msg", "monitoring logger for errors")

		select {
		case <-u.stopChan:
			level.Debug(u.config.Logger).Log("msg", "updater stop requested")
			return
		case <-time.After(u.monitorInterval):
			if u.runFallback {
				continue
			}

			currentErrorCount := u.updater.ErrorCount()
			if currentErrorCount > errorThreshold {
				// Too many errors. Stop the new updater and fall back to the old one.
				u.stopExecution()
				u.runFallback = true
				u.startUpdater()
			}
		}
	}
}

func (u *updaterCmd) startUpdater() error {
	// Failing to start the updater is not a fatal launcher
	// error. If there's a problem, sleep and try
	// again. Implementing this is a bit gnarly. In the event of a
	// success, we get a nil error, and a stop function. But I don't
	// see a simple way to ensure the updater is still running in
	// the background.
	for {
		level.Debug(u.config.Logger).Log("msg", "updater starting", "is_fallback", u.runFallback)

		// run the updater and set the stop function so that the interrupt has access to it
		var stop func()
		var err error
		if u.runFallback {
			stop, err = u.fallbackUpdater.Run(tuf.WithFrequency(u.config.AutoupdateInterval), tuf.WithLogger(u.config.Logger))
		} else {
			stop, err = u.updater.Run(tuf.WithFrequency(u.config.AutoupdateInterval), tuf.WithLogger(u.config.Logger))
		}

		u.stopExecution = stop
		if err == nil {
			// Successfully started
			return nil
		}

		// err != nil, log it and loop again
		level.Error(u.config.Logger).Log("msg", "error running updater", "err", err)
		select {
		case <-u.stopChan:
			level.Debug(u.config.Logger).Log("msg", "updater stop requested, breaking loop")
			return err
		case <-time.After(u.runUpdaterRetryInterval):
			continue
		}
	}
}

func (u *updaterCmd) interrupt(err error) {

	level.Info(u.config.Logger).Log("msg", "updater interrupted", "err", err)

	// non-blocking channel send
	select {
	case u.stopChan <- true:
		level.Info(u.config.Logger).Log("msg", "updater interrupt sent signal over stop channel")
	default:
		level.Info(u.config.Logger).Log("msg", "updater interrupt without sending signal over stop channel (no one to receive)")
	}

	if u.stopExecution != nil {
		u.stopExecution()
	}
}

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
	"github.com/kolide/launcher/pkg/tuf"
	legacytuf "github.com/kolide/updater/tuf"
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

	// create the legacy updater
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

	// create the new tuf
	tufAutoupdater, err := tuf.NewTufAutoupdater(
		config.TufServerURL,
		binaryPath,
		config.RootDirectory,
		tuf.WithLogger(config.Logger),
		tuf.WithChannel(tuf.DefaultChannel),
		tuf.WithUpdateCheckInterval(config.AutoupdateInterval),
	)
	if err != nil {
		// Log the error, but don't return it -- the new TUF autoupdater is not critical
		level.Debug(config.Logger).Log("msg", "could not create TUF autoupdater", "err", err)
	}

	updateCmd := &updaterCmd{
		updater:                 updater,
		tufAutoupdater:          tufAutoupdater,
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
	Run(opts ...legacytuf.Option) (stop func(), err error)
	RollingErrorCount() int
}

type updaterCmd struct {
	updater                 updater
	tufAutoupdater          updater
	ctx                     context.Context
	stopChan                chan bool
	stopExecution           func()
	config                  *UpdaterConfig
	runUpdaterRetryInterval time.Duration
	monitorInterval         time.Duration
}

const allowableDailyErrorCountThreshold = 4

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

	// Failing to start the updater is not a fatal launcher
	// error. If there's a problem, sleep and try
	// again. Implementing this is a bit gnarly. In the event of a
	// success, we get a nil error, and a stop function. But I don't
	// see a simple way to ensure the updater is still running in
	// the background.
	for {
		level.Debug(u.config.Logger).Log("msg", "updater starting")

		// run the updater and set the stop function so that the interrupt has access to it
		stop, err := u.updater.Run(legacytuf.WithFrequency(u.config.AutoupdateInterval), legacytuf.WithLogger(u.config.Logger))
		u.stopExecution = stop
		if err == nil {
			break
		}

		// err != nil, log it and loop again
		level.Error(u.config.Logger).Log("msg", "error running updater", "err", err)
		select {
		case <-u.stopChan:
			level.Debug(u.config.Logger).Log("msg", "updater stop requested, breaking loop")
			return nil
		case <-time.After(u.runUpdaterRetryInterval):
			break
		}
	}

	go u.runAndMonitorTufAutoupdater()

	level.Debug(u.config.Logger).Log("msg", "updater waiting ... just sitting until done signal")
	<-u.ctx.Done()

	return nil
}

func (u *updaterCmd) runAndMonitorTufAutoupdater() {
	if u.tufAutoupdater == nil {
		level.Debug(u.config.Logger).Log("msg", "TUF autoupdater was not initialized, cannot run and monitor it")
		return
	}

	// All the TUF autoupdater does right now is maintain a local TUF repo; it does not download and stage updates yet.
	stop, err := u.tufAutoupdater.Run(legacytuf.WithFrequency(u.config.AutoupdateInterval), legacytuf.WithLogger(u.config.Logger))
	if err != nil {
		level.Debug(u.config.Logger).Log("msg", "could not run new TUF autoupdater", "err", err)
		return
	}

	// Check the new autoupdater periodically for errors
	for {
		level.Debug(u.config.Logger).Log("msg", "checking TUF autoupdater for errors")

		select {
		case <-u.stopChan:
			level.Debug(u.config.Logger).Log("msg", "TUF autoupdater stop requested")
			stop()
			return
		case <-time.After(u.monitorInterval):
			currentErrorCount := u.tufAutoupdater.RollingErrorCount()
			if currentErrorCount > allowableDailyErrorCountThreshold {
				// Error count over threshold -- log
				level.Debug(u.config.Logger).Log("msg", "TUF autoupdater error count over threshold", "error_count", currentErrorCount)
			}
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

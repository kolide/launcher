package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/service"
)

const (
	// How long to wait before erroring because we cannot open the osquery
	// extension socket.
	socketOpenTimeout = 10 * time.Second

	// How often to try to open the osquery extension socket
	socketOpenInterval = 200 * time.Millisecond

	// How frequently we should healthcheck the client/server
	healthCheckInterval = 60 * time.Second

	// The maximum amount of time to wait for the osquery socket to be available -- overrides context deadline
	maxSocketWaitTime = 30 * time.Second
)

type Runner struct {
	instance      *OsqueryInstance
	instanceLock  sync.Mutex
	slogger       *slog.Logger
	knapsack      types.Knapsack
	serviceClient service.KolideService
	shutdown      chan struct{}
	interrupted   bool
	opts          []OsqueryInstanceOption
}

func New(k types.Knapsack, serviceClient service.KolideService, opts ...OsqueryInstanceOption) *Runner {
	runner := &Runner{
		instance:      newInstance(k, serviceClient, opts...),
		slogger:       k.Slogger().With("component", "osquery_runner"),
		knapsack:      k,
		serviceClient: serviceClient,
		shutdown:      make(chan struct{}),
		opts:          opts,
	}

	k.RegisterChangeObserver(runner,
		keys.WatchdogEnabled, keys.WatchdogMemoryLimitMB, keys.WatchdogUtilizationLimitPercent, keys.WatchdogDelaySec,
	)

	return runner
}

func (r *Runner) Run() error {
	// Ensure we don't try to restart the instance before it's launched
	r.instanceLock.Lock()
	if err := r.instance.Launch(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelWarn,
			"failed to launch osquery instance",
			"err", err,
		)
		r.instanceLock.Unlock()
		return fmt.Errorf("starting instance: %w", err)
	}
	r.instanceLock.Unlock()

	// This loop waits for the completion of the async routines,
	// and either restarts the instance (if Shutdown was not
	// called), or stops (if Shutdown was called).
	for {
		// Wait for async processes to exit
		<-r.instance.Exited()
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"osquery instance exited",
		)

		select {
		case <-r.shutdown:
			// Intentional shutdown, this loop can exit
			if err := r.instance.stats.Exited(nil); err != nil {
				r.slogger.Log(context.TODO(), slog.LevelWarn,
					"error recording osquery instance exit to history",
					"err", err,
				)
			}
			return nil
		default:
			// Don't block
		}

		// Error case -- osquery instance shut down and needs to be restarted
		err := r.instance.WaitShutdown()
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"unexpected restart of instance",
			"err", err,
		)

		if err := r.instance.stats.Exited(err); err != nil {
			r.slogger.Log(context.TODO(), slog.LevelWarn,
				"error recording osquery instance exit to history",
				"err", err,
			)
		}

		r.instanceLock.Lock()
		r.instance = newInstance(r.knapsack, r.serviceClient, r.opts...)
		if err := r.instance.Launch(); err != nil {
			r.slogger.Log(context.TODO(), slog.LevelWarn,
				"fatal error restarting instance, shutting down",
				"err", err,
			)
			r.instanceLock.Unlock()
			if err := r.Shutdown(); err != nil {
				r.slogger.Log(context.TODO(), slog.LevelWarn,
					"could not perform shutdown",
					"err", err,
				)
			}

			// Failed to restart instance -- exit rungroup so launcher can reload
			return fmt.Errorf("restarting instance after unexpected exit: %w", err)
		}

		r.instanceLock.Unlock()
	}
}

func (r *Runner) Query(query string) ([]map[string]string, error) {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	return r.instance.Query(query)
}

func (r *Runner) Interrupt(_ error) {
	if err := r.Shutdown(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not shut down runner on interrupt",
			"err", err,
		)
	}
}

// Shutdown instructs the runner to permanently stop the running instance (no
// restart will be attempted).
func (r *Runner) Shutdown() error {
	if r.interrupted {
		// Already shut down, nothing else to do
		return nil
	}

	r.interrupted = true
	close(r.shutdown)
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	r.instance.BeginShutdown()
	if err := r.instance.WaitShutdown(); err != context.Canceled && err != nil {
		return fmt.Errorf("while shutting down instance: %w", err)
	}
	return nil
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface -- handles updates to flags
// that we care about, which are enable_watchdog, watchdog_delay_sec, watchdog_memory_limit_mb,
// and watchdog_utilization_limit_percent.
func (r *Runner) FlagsChanged(flagKeys ...keys.FlagKey) {
	r.slogger.Log(context.TODO(), slog.LevelDebug,
		"control server flags changed, restarting instance to apply",
		"flags", fmt.Sprintf("%+v", flagKeys),
	)

	if err := r.Restart(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelError,
			"could not restart osquery instance after flag change",
			"err", err,
		)
	}
}

// Ping satisfies the control.subscriber interface -- the runner subscribes to changes to
// the katc_config subsystem.
func (r *Runner) Ping() {
	r.slogger.Log(context.TODO(), slog.LevelDebug,
		"KATC configuration changed, restarting instance to apply",
	)

	if err := r.Restart(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelError,
			"could not restart osquery instance after KATC configuration changed",
			"err", err,
		)
	}
}

// Restart allows you to cleanly shutdown the current instance and launch a new
// instance with the same configurations.
func (r *Runner) Restart() error {
	r.slogger.Log(context.TODO(), slog.LevelDebug,
		"runner.Restart called",
	)
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	// Cancelling will cause all of the cleanup routines to execute, and a
	// new instance will start.
	r.instance.BeginShutdown()
	r.instance.WaitShutdown()

	return nil
}

// Healthy checks the health of the instance and returns an error describing
// any problem.
func (r *Runner) Healthy() error {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	return r.instance.Healthy()
}

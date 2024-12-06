package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/service"
	"golang.org/x/sync/errgroup"
)

const (
	launchRetryDelay = 30 * time.Second
)

type Runner struct {
	registrationIds []string                    // we expect to run one instance per registration ID
	instances       map[string]*OsqueryInstance // maps registration ID to currently-running instance
	instanceLock    sync.Mutex                  // locks access to `instances` to avoid e.g. restarting an instance that isn't running yet
	slogger         *slog.Logger
	knapsack        types.Knapsack
	serviceClient   service.KolideService   // shared service client for communication between osquery instance and Kolide SaaS
	opts            []OsqueryInstanceOption // global options applying to all osquery instances
	shutdown        chan struct{}
	interrupted     bool
}

func New(k types.Knapsack, serviceClient service.KolideService, opts ...OsqueryInstanceOption) *Runner {
	runner := &Runner{
		registrationIds: k.RegistrationIds(),
		instances:       make(map[string]*OsqueryInstance),
		slogger:         k.Slogger().With("component", "osquery_runner"),
		knapsack:        k,
		serviceClient:   serviceClient,
		shutdown:        make(chan struct{}),
		opts:            opts,
	}

	k.RegisterChangeObserver(runner,
		keys.WatchdogEnabled, keys.WatchdogMemoryLimitMB, keys.WatchdogUtilizationLimitPercent, keys.WatchdogDelaySec,
	)

	return runner
}

func (r *Runner) Run() error {
	// Create a group to track the workers running each instance
	wg, ctx := errgroup.WithContext(context.Background())

	// Start each worker for each instance
	for _, registrationId := range r.registrationIds {
		id := registrationId
		wg.Go(func() error {
			if err := r.runInstance(id); err != nil {
				r.slogger.Log(ctx, slog.LevelWarn,
					"runner terminated running osquery instance unexpectedly, shutting down runner",
					"err", err,
				)

				if err := r.Shutdown(); err != nil {
					r.slogger.Log(ctx, slog.LevelError,
						"could not shut down runner after failure to run osquery instance",
						"err", err,
					)
				}
				return err
			}

			return nil
		})
	}

	// Wait for all workers to exit
	if err := wg.Wait(); err != nil {
		return fmt.Errorf("running osquery instances: %w", err)
	}

	return nil
}

// runInstance starts a worker that launches the instance for the given registration ID, and
// then ensures that instance stays up. It exits if `Shutdown` is called, or if the instance
// exits and cannot be restarted.
func (r *Runner) runInstance(registrationId string) error {
	slogger := r.slogger.With("registration_id", registrationId)

	// First, launch the instance.
	instance, err := r.launchInstanceWithRetries(registrationId)
	if err != nil {
		// We only receive an error on launch if the runner has been shut down -- in that case,
		// return now.
		return fmt.Errorf("starting instance for %s: %w", registrationId, err)
	}

	// This loop restarts the instance as necessary. It exits when `Shutdown` is called,
	// or if the instance exits and cannot be restarted.
	for {
		<-instance.Exited()
		slogger.Log(context.TODO(), slog.LevelInfo,
			"osquery instance exited",
		)

		select {
		case <-r.shutdown:
			// Intentional shutdown of runner -- exit worker
			return nil
		default:
			// Continue on to restart the instance
		}

		// The osquery instance either exited on its own, or we called `Restart`.
		// Either way, we wait for exit to complete, and then restart the instance.
		err := instance.WaitShutdown()
		slogger.Log(context.TODO(), slog.LevelInfo,
			"unexpected restart of instance",
			"err", err,
		)

		var launchErr error
		instance, launchErr = r.launchInstanceWithRetries(registrationId)
		if launchErr != nil {
			// We only receive an error on launch if the runner has been shut down -- in that case,
			// return now.
			return fmt.Errorf("restarting instance for %s after unexpected exit: %w", registrationId, launchErr)
		}
	}
}

// launchInstanceWithRetries repeatedly tries to create and launch a new osquery instance.
// It will retry until it succeeds, or until the runner is shut down.
func (r *Runner) launchInstanceWithRetries(registrationId string) (*OsqueryInstance, error) {
	for {
		// Lock to ensure we don't try to restart before launch is complete.
		r.instanceLock.Lock()
		instance := newInstance(registrationId, r.knapsack, r.serviceClient, r.opts...)
		err := instance.Launch()

		// Success!
		if err == nil {
			// Now that the instance is running, we can add it to `r.instances` and remove the lock
			r.instances[registrationId] = instance
			r.instanceLock.Unlock()

			r.slogger.Log(context.TODO(), slog.LevelInfo,
				"runner successfully launched instance",
				"registration_id", registrationId,
			)

			return instance, nil
		}

		// Launching was not successful. Unlock, log the error, and wait to retry.
		r.instanceLock.Unlock()
		r.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not launch instance, will retry after delay",
			"err", err,
			"registration_id", registrationId,
		)

		select {
		case <-r.shutdown:
			return nil, fmt.Errorf("runner received shutdown, halting before successfully launching instance for %s", registrationId)
		case <-time.After(launchRetryDelay):
			// Continue to retry
			continue
		}
	}
}

func (r *Runner) Query(query string) ([]map[string]string, error) {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	// For now, grab the default (i.e. only) instance
	instance, ok := r.instances[types.DefaultRegistrationId]
	if !ok {
		return nil, errors.New("no default instance exists, cannot query")
	}

	return instance.Query(query)
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

	if err := r.triggerShutdownForInstances(); err != nil {
		return fmt.Errorf("triggering shutdown for instances during runner shutdown: %w", err)
	}

	return nil
}

// triggerShutdownForInstances asks all instances in `r.instances` to shut down.
func (r *Runner) triggerShutdownForInstances() error {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	// Shut down the instances in parallel
	shutdownWg, _ := errgroup.WithContext(context.Background())
	for registrationId, instance := range r.instances {
		id := registrationId
		i := instance
		shutdownWg.Go(func() error {
			i.BeginShutdown()
			if err := i.WaitShutdown(); err != context.Canceled && err != nil {
				return fmt.Errorf("shutting down instance %s: %w", id, err)
			}
			return nil
		})
	}

	if err := shutdownWg.Wait(); err != nil {
		return fmt.Errorf("shutting down all instances: %+v", err)
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

	// Shut down the instances -- this will trigger a restart in each `runInstance`.
	if err := r.triggerShutdownForInstances(); err != nil {
		return fmt.Errorf("triggering shutdown for instances during runner restart: %w", err)
	}

	return nil
}

// Healthy checks the health of the instance and returns an error describing
// any problem.
func (r *Runner) Healthy() error {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	healthcheckErrs := make([]error, 0)
	for _, registrationId := range r.registrationIds {
		instance, ok := r.instances[registrationId]
		if !ok {
			healthcheckErrs = append(healthcheckErrs, fmt.Errorf("running instance does not exist for %s", registrationId))
			continue
		}

		if err := instance.Healthy(); err != nil {
			healthcheckErrs = append(healthcheckErrs, fmt.Errorf("healthcheck error for %s: %w", registrationId, err))
		}
	}

	if len(healthcheckErrs) > 0 {
		return fmt.Errorf("healthchecking all instances: %+v", healthcheckErrs)
	}

	return nil
}

func (r *Runner) InstanceStatuses() map[string]types.InstanceStatus {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	instanceStatuses := make(map[string]types.InstanceStatus)
	for _, registrationId := range r.registrationIds {
		instance, ok := r.instances[registrationId]
		if !ok {
			instanceStatuses[registrationId] = types.InstanceStatusNotStarted
			continue
		}

		if err := instance.Healthy(); err != nil {
			instanceStatuses[registrationId] = types.InstanceStatusUnhealthy
			continue
		}

		instanceStatuses[registrationId] = types.InstanceStatusHealthy
	}

	return instanceStatuses
}

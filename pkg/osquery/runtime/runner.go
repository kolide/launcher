package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/traces"
	"golang.org/x/sync/errgroup"
)

const (
	launchRetryDelay = 10 * time.Second
)

// settingsStoreWriter writes to our startup settings store
type settingsStoreWriter interface {
	WriteSettings() error
}

type Runner struct {
	registrationIds []string                    // we expect to run one instance per registration ID
	regIDLock       sync.Mutex                  // locks access to registrationIds
	instances       map[string]*OsqueryInstance // maps registration ID to currently-running instance
	instanceLock    sync.Mutex                  // locks access to `instances` to avoid e.g. restarting an instance that isn't running yet
	slogger         *slog.Logger
	knapsack        types.Knapsack
	serviceClient   service.KolideService   // shared service client for communication between osquery instance and Kolide SaaS
	settingsWriter  settingsStoreWriter     // writes to startup settings store
	opts            []OsqueryInstanceOption // global options applying to all osquery instances
	shutdown        chan struct{}           // buffered shutdown channel for to enable shutting down to restart or exit
	rerunRequired   atomic.Bool
	interrupted     atomic.Bool
}

func New(k types.Knapsack, serviceClient service.KolideService, settingsWriter settingsStoreWriter, opts ...OsqueryInstanceOption) *Runner {
	runner := &Runner{
		registrationIds: k.RegistrationIDs(),
		instances:       make(map[string]*OsqueryInstance),
		slogger:         k.Slogger().With("component", "osquery_runner"),
		knapsack:        k,
		serviceClient:   serviceClient,
		settingsWriter:  settingsWriter,
		// the buffer length is arbitrarily set at 100, this number just needs to be higher than the total possible instances
		shutdown: make(chan struct{}, 100),
		opts:     opts,
	}

	k.RegisterChangeObserver(runner,
		keys.WatchdogEnabled, keys.WatchdogMemoryLimitMB, keys.WatchdogUtilizationLimitPercent, keys.WatchdogDelaySec,
	)

	return runner
}

// String method is only added to runner because it is often used in our runtime tests as an argument
// passed to mocked knapsack calls. when we AssertExpectations, the runner struct is traversed by the
// Diff logic inside testify. This causes data races to be incorrectly reported for structs containing mutexes
// (the second read is coming from testify). Implementing the stringer interface with locks acquired prevents these
// races. see (one of) the issues here for additional context https://github.com/stretchr/testify/issues/1597
func (r *Runner) String() string {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	r.regIDLock.Lock()
	defer r.regIDLock.Unlock()

	return fmt.Sprintf("%#v", r)
}

func (r *Runner) Run() error {
	for {
		err := r.runRegisteredInstances()
		if err != nil {
			// log any errors but continue, in case we intend to reload
			r.slogger.Log(context.TODO(), slog.LevelWarn,
				"runRegisteredInstances terminated with error",
				"err", err,
			)
		}

		// if we're in a state that required re-running all registered instances,
		// reset the field and do that
		if r.rerunRequired.Load() {
			r.rerunRequired.Store(false)
			continue
		}

		return err
	}
}

func (r *Runner) runRegisteredInstances() error {
	// clear the internal instances to add back in fresh as we runInstance,
	// this prevents old instances from sticking around if a registrationID is ever removed
	r.instanceLock.Lock()
	r.instances = make(map[string]*OsqueryInstance)
	r.instanceLock.Unlock()

	// Create a group to track the workers running each instance
	wg, ctx := errgroup.WithContext(context.TODO())

	// Start each worker for each instance
	r.regIDLock.Lock()
	regIDs := r.registrationIds
	r.regIDLock.Unlock()

	for _, registrationId := range regIDs {
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
	ctx := context.TODO()

	// First, launch the instance.
	instance, err := r.launchInstanceWithRetries(ctx, registrationId)
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
		err := instance.WaitShutdown(ctx)
		slogger.Log(context.TODO(), slog.LevelInfo,
			"unexpected restart of instance",
			"err", err,
		)

		var launchErr error
		instance, launchErr = r.launchInstanceWithRetries(ctx, registrationId)
		if launchErr != nil {
			// We only receive an error on launch if the runner has been shut down -- in that case,
			// return now.
			return fmt.Errorf("restarting instance for %s after unexpected exit: %w", registrationId, launchErr)
		}
	}
}

// launchInstanceWithRetries repeatedly tries to create and launch a new osquery instance.
// It will retry until it succeeds, or until the runner is shut down.
func (r *Runner) launchInstanceWithRetries(ctx context.Context, registrationId string) (*OsqueryInstance, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	for {
		// Add the instance to our instances map right away, so that if we receive a shutdown
		// request during launch, we can shut down the instance.
		r.instanceLock.Lock()
		instance := newInstance(registrationId, r.knapsack, r.serviceClient, r.settingsWriter, r.opts...)
		r.instances[registrationId] = instance
		r.instanceLock.Unlock()
		err := instance.Launch()

		// Success!
		if err == nil {
			r.slogger.Log(ctx, slog.LevelInfo,
				"runner successfully launched instance",
				"registration_id", registrationId,
			)

			return instance, nil
		}

		// Launching was not successful. Shut down the instance, log the error, and wait to retry.
		r.slogger.Log(ctx, slog.LevelWarn,
			"could not launch instance, will retry after delay",
			"err", err,
			"registration_id", registrationId,
		)
		instance.BeginShutdown()
		if err := instance.WaitShutdown(ctx); err != context.Canceled && err != nil {
			r.slogger.Log(ctx, slog.LevelWarn,
				"error shutting down instance that failed to launch",
				"err", err,
				"registration_id", registrationId,
			)
		}

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
	instance, ok := r.instances[types.DefaultRegistrationID]
	if !ok {
		return nil, errors.New("no default instance exists, cannot query")
	}

	return instance.Query(query)
}

func (r *Runner) Interrupt(_ error) {
	if r.interrupted.Load() {
		// Already shut down, nothing else to do
		return
	}

	r.interrupted.Store(true)

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
	ctx, span := traces.StartSpan(context.TODO())
	defer span.End()
	// ensure one shutdown is sent for each instance to read
	r.instanceLock.Lock()
	for range r.instances {
		r.shutdown <- struct{}{}
	}
	r.instanceLock.Unlock()

	if err := r.triggerShutdownForInstances(ctx); err != nil {
		return fmt.Errorf("triggering shutdown for instances during runner shutdown: %w", err)
	}

	return nil
}

// triggerShutdownForInstances asks all instances in `r.instances` to shut down.
func (r *Runner) triggerShutdownForInstances(ctx context.Context) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	// Shut down the instances in parallel
	shutdownWg, ctx := errgroup.WithContext(ctx)
	for registrationId, instance := range r.instances {
		id := registrationId
		i := instance
		shutdownWg.Go(func() error {
			i.BeginShutdown()
			if err := i.WaitShutdown(ctx); err != context.Canceled && err != nil {
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
func (r *Runner) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	r.slogger.Log(ctx, slog.LevelDebug,
		"control server flags changed, restarting instance to apply",
		"flags", fmt.Sprintf("%+v", flagKeys),
	)

	if err := r.Restart(ctx); err != nil {
		r.slogger.Log(ctx, slog.LevelError,
			"could not restart osquery instance after flag change",
			"err", err,
		)
	}
}

// Ping satisfies the control.subscriber interface -- the runner subscribes to changes to
// the katc_config subsystem.
func (r *Runner) Ping() {
	ctx, span := traces.StartSpan(context.TODO())
	defer span.End()

	r.slogger.Log(ctx, slog.LevelDebug,
		"KATC configuration changed, restarting instance to apply",
	)

	if err := r.Restart(ctx); err != nil {
		r.slogger.Log(ctx, slog.LevelError,
			"could not restart osquery instance after KATC configuration changed",
			"err", err,
		)
	}
}

// Restart allows you to cleanly shutdown the current instance and launch a new
// instance with the same configurations.
func (r *Runner) Restart(ctx context.Context) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	r.slogger.Log(ctx, slog.LevelDebug,
		"runner.Restart called",
	)

	// Shut down the instances -- this will trigger a restart in each `runInstance`.
	if err := r.triggerShutdownForInstances(ctx); err != nil {
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
	r.regIDLock.Lock()
	regIDs := r.registrationIds
	r.regIDLock.Unlock()

	for _, registrationId := range regIDs {
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

	r.regIDLock.Lock()
	regIDs := r.registrationIds
	r.regIDLock.Unlock()
	instanceStatuses := make(map[string]types.InstanceStatus)
	for _, registrationId := range regIDs {
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

// UpdateRegistrationIDs detects any changes between the new and stored registration IDs,
// and resets the runner instances for the new registrationIDs if required
func (r *Runner) UpdateRegistrationIDs(newRegistrationIDs []string) error {
	slices.Sort(newRegistrationIDs)

	r.regIDLock.Lock()
	existingRegistrationIDs := r.registrationIds
	r.regIDLock.Unlock()
	slices.Sort(existingRegistrationIDs)

	if slices.Equal(newRegistrationIDs, existingRegistrationIDs) {
		r.slogger.Log(context.TODO(), slog.LevelDebug,
			"skipping runner restarts for updated registration IDs, no changes detected",
		)

		return nil
	}

	r.slogger.Log(context.TODO(), slog.LevelDebug,
		"detected changes to registrationIDs, will restart runner instances",
		"previous_registration_ids", existingRegistrationIDs,
		"new_registration_ids", newRegistrationIDs,
	)

	// we know there are changes, safe to update the internal registrationIDs now
	r.regIDLock.Lock()
	r.registrationIds = newRegistrationIDs
	r.regIDLock.Unlock()
	// mark rerun as required so that we can safely shutdown all workers and have the changes
	// picked back up from within the main Run function
	r.rerunRequired.Store(true)

	if err := r.Shutdown(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not shut down runner instances for restart after registration changes",
			"err", err,
		)

		return err
	}

	return nil
}

package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
	"golang.org/x/sync/errgroup"
)

const (
	launchRetryDelay = 10 * time.Second
)

const (
	runnerUnstarted int32 = iota
	runnerRunning
	runnerShutdown
)

// settingsStoreWriter writes to our startup settings store
type settingsStoreWriter interface {
	WriteSettings() error
}

type wrappedInstance struct {
	*OsqueryInstance
	cancel func()
}

type Runner struct {
	enrollmentIds    []string                   // we expect to run one instance per enrollment ID
	instances        map[string]wrappedInstance // maps enrollment ID to currently-running instance
	instanceLock     sync.Mutex                 // locks access to `instances` to avoid e.g. restarting an instance that isn't running yet
	slogger          *slog.Logger
	knapsack         types.Knapsack
	logPublishClient types.OsqueryPublisher  // client used for cutting over to new osquery log publication service (agent-ingester)
	settingsWriter   settingsStoreWriter     // writes to startup settings store
	opts             []OsqueryInstanceOption // global options applying to all osquery instances
	shutdown         chan struct{}           // closed by Shutdown to stop all instances
	done             chan struct{}           // closed when all instances have stopped
	state            atomic.Int32            // runnerUnstarted -> runnerRunning -> runnerShutdown
	needsRestart     *atomic.Bool
	restartLock      sync.Mutex // use a restart lock to ensure we don't get multiple quick succession restarts due to in modern standy flapping
}

func New(k types.Knapsack, logPublishClient types.OsqueryPublisher, settingsWriter settingsStoreWriter, opts ...OsqueryInstanceOption) *Runner {
	runner := &Runner{
		enrollmentIds:    k.EnrollmentIDs(),
		instances:        make(map[string]wrappedInstance),
		slogger:          k.Slogger().With("component", "osquery_runner"),
		knapsack:         k,
		logPublishClient: logPublishClient,
		settingsWriter:   settingsWriter,
		shutdown:         make(chan struct{}),
		done:             make(chan struct{}),
		opts:             opts,
		needsRestart:     &atomic.Bool{},
	}

	k.RegisterChangeObserver(runner,
		keys.WatchdogEnabled,
		keys.WatchdogMemoryLimitMB,
		keys.WatchdogUtilizationLimitPercent,
		keys.WatchdogDelaySec,
		keys.InModernStandby, // we delay restarts while in modern standby, so we need to be notified of changes to perform restarts on wake
	)

	return runner
}

func (r *Runner) Run() error {
	if !r.state.CompareAndSwap(runnerUnstarted, runnerRunning) {
		// Shut down before we were ever scheduled -- nothing to run
		return nil
	}
	defer close(r.done)

	stopCtx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go func() {
		select {
		case <-r.shutdown:
			cancel()
		case <-stopCtx.Done():
		}
	}()

	// Create a group to track the workers running each instance
	wg, ctx := errgroup.WithContext(stopCtx)

	// Start each worker for each instance
	for _, enrollmentId := range r.enrollmentIds {
		id := enrollmentId
		wg.Go(func() error {
			return r.runInstance(ctx, id)
		})
	}

	// Wait for all workers to exit
	if err := wg.Wait(); err != nil {
		return fmt.Errorf("running osquery instances: %w", err)
	}

	return nil
}

// runInstance repeatedly runs the instance for the given enrollment ID, registering
// each in r.instances. It returns only when ctx is cancelled.
func (r *Runner) runInstance(ctx context.Context, enrollmentId string) error {
	slogger := r.slogger.With("enrollment_id", enrollmentId)

	for ctx.Err() == nil {
		// Add the instance to our instances map right away, so that if we receive a shutdown
		// request during launch, we can shut down the instance.
		instanceCtx, instanceCancel := context.WithCancel(ctx)
		instance := newInstance(enrollmentId, r.knapsack, r.logPublishClient, r.settingsWriter, r.opts...)
		r.instanceLock.Lock()
		r.instances[enrollmentId] = wrappedInstance{
			OsqueryInstance: instance,
			cancel:          instanceCancel,
		}
		r.instanceLock.Unlock()

		err := instance.Launch(instanceCtx)
		instanceCancel()

		observability.OsqueryRestartCounter.Add(ctx, 1)
		slogger.Log(ctx, slog.LevelInfo,
			"osquery instance exited, will restart after delay",
			"err", err,
		)

		// Instances for an enrollment ID share a database path, and a predecessor may not have
		// released its locks yet (see calculateOsqueryPaths) -- space launches out.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(launchRetryDelay):
			// Continue to relaunch
		}
	}

	return nil
}

func (r *Runner) Query(query string) ([]map[string]string, error) {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	// For now, grab the default (i.e. only) instance
	instance, ok := r.instances[types.DefaultEnrollmentID]
	if !ok {
		return nil, errors.New("no default instance exists, cannot query")
	}

	return instance.Query(query)
}

func (r *Runner) Interrupt(_ error) {
	r.Shutdown()
}

// Shutdown instructs the runner to permanently stop the running instance (no
// restart will be attempted).
func (r *Runner) Shutdown() {
	_, span := observability.StartSpan(context.TODO())
	defer span.End()

	switch r.state.Swap(runnerShutdown) {
	case runnerRunning:
		close(r.shutdown)
		<-r.done
	case runnerUnstarted:
		// rungroup may interrupt us before Run is ever scheduled. The Swap above
		// guarantees Run's CompareAndSwap fails, so nothing will launch.
	case runnerShutdown:
		// Already shut down, nothing else to do
	}
}

// restartInstances cancels all running instances' contexts
func (r *Runner) restartInstances() {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	for _, instance := range r.instances {
		instance.cancel()
	}
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface -- handles updates to flags
// that we care about, which are enable_watchdog, watchdog_delay_sec, watchdog_memory_limit_mb, in_modern_standby,
// and watchdog_utilization_limit_percent.
func (r *Runner) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	r.restartLock.Lock()

	// only modern standby changed and no restart pending
	if len(flagKeys) == 1 && flagKeys[0] == keys.InModernStandby && !r.needsRestart.Load() {
		r.slogger.Log(ctx, slog.LevelDebug,
			"only modern standby flag changed and no restart needed, doing nothing",
			"in_modern_standby", r.knapsack.InModernStandby(),
			"needs_restart", r.needsRestart.Load(),
		)

		r.restartLock.Unlock()
		return
	}

	r.restartLock.Unlock()

	r.slogger.Log(ctx, slog.LevelDebug,
		"restarting osquery instances due to flag changes or needed restart",
		"flags", fmt.Sprintf("%+v", flagKeys),
		"in_modern_standby", r.knapsack.InModernStandby(),
		"needs_restart", r.needsRestart.Load(),
	)

	// r.Restart will check if we are in modern standby and set the needsRestart flag if so and we will restart
	// when we exit modern standby
	if err := r.Restart(ctx); err != nil {
		r.slogger.Log(ctx, slog.LevelError,
			"could not restart osquery instance after flag change or needed restart",
			"err", err,
		)
	}
}

// Ping satisfies the control.subscriber interface -- the runner subscribes to changes to
// the katc_config subsystem.
func (r *Runner) Ping() {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	r.instanceLock.Lock()
	updatedInPlace := true
	for _, instance := range r.instances {
		if err := instance.ReloadKatcExtension(ctx); err != nil {
			r.slogger.Log(ctx, slog.LevelInfo,
				"could not update KATC info in-place -- must restart instances to apply",
				"err", err,
			)
			updatedInPlace = false
			break
		}
	}
	r.instanceLock.Unlock()

	if updatedInPlace {
		r.slogger.Log(ctx, slog.LevelDebug,
			"KATC configuration changed, successfully updated in-place without restarting instances",
		)
		return
	}

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
// If we are in modern standby, we will not restart now, but will instead
// set a flag to indicate that we should restart when we exit modern standby.
func (r *Runner) Restart(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	r.restartLock.Lock()
	defer r.restartLock.Unlock()

	r.slogger.Log(ctx, slog.LevelDebug,
		"runner.Restart called",
	)

	// check to see if we are in modern standby; if so, we cannot restart now
	// when we exit modern standby, we'll get a call to FlagsChanged which will
	// trigger a restart then
	if r.knapsack.InModernStandby() {
		r.slogger.Log(ctx, slog.LevelInfo,
			"device is in modern standby, not restarting osquery instances, will apply changes on wake",
		)

		r.needsRestart.Store(true)
		return nil
	}

	r.needsRestart.Store(false)

	// Stop the instances -- this will trigger a relaunch in each `runInstance`.
	r.restartInstances()

	return nil
}

// Healthy checks the health of the instance and returns an error describing
// any problem.
func (r *Runner) Healthy() error {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	healthcheckErrs := make([]error, 0)
	for _, enrollmentId := range r.enrollmentIds {
		instance, ok := r.instances[enrollmentId]
		if !ok {
			healthcheckErrs = append(healthcheckErrs, fmt.Errorf("running instance does not exist for %s", enrollmentId))
			continue
		}

		if err := instance.Healthy(); err != nil {
			healthcheckErrs = append(healthcheckErrs, fmt.Errorf("healthcheck error for %s: %w", enrollmentId, err))
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
	for _, enrollmentId := range r.enrollmentIds {
		instance, ok := r.instances[enrollmentId]
		if !ok {
			instanceStatuses[enrollmentId] = types.InstanceStatusNotStarted
			continue
		}

		if err := instance.Healthy(); err != nil {
			instanceStatuses[enrollmentId] = types.InstanceStatusUnhealthy
			continue
		}

		instanceStatuses[enrollmentId] = types.InstanceStatusHealthy
	}

	return instanceStatuses
}

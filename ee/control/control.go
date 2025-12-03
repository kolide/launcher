package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"go.uber.org/atomic"
	"golang.org/x/exp/slices"
)

const ForceFullControlDataFetchAction = "force_full_control_data_fetch"

// ControlService is the main object that manages the control service. It is responsible for fetching
// and caching control data, and updating consumers and subscribers.
type ControlService struct {
	slogger         *slog.Logger
	knapsack        types.Knapsack
	cancel          context.CancelFunc
	requestInterval *atomic.Duration
	requestTicker   *time.Ticker
	fetcher         dataProvider
	fetchMutex      sync.Mutex
	fetchFull       bool
	fetchFullMutex  sync.Mutex
	store           types.GetterSetter
	lastFetched     map[string]string
	consumers       map[string]consumer
	subscribers     map[string][]subscriber
}

// consumer is an interface for something that consumes control server data updates. The
// control server supports at most one consumer per subsystem.
type consumer interface {
	Update(data io.Reader) error
}

// subscriber is an interface for something that wants to be notified when a subsystem has been updated.
// Subscribers do not receive data -- they are expected to read the data from where consumers write it.
type subscriber interface {
	Ping()
}

// dataProvider is an interface for something that can retrieve control data. Authentication, HTTP,
// file system access, etc. lives below this abstraction layer.
type dataProvider interface {
	GetConfig(ctx context.Context) (io.Reader, error)
	GetSubsystemData(ctx context.Context, hash string) (io.Reader, error)
	SendMessage(ctx context.Context, method string, params interface{}) error
}

func New(k types.Knapsack, fetcher dataProvider, opts ...Option) *ControlService {
	cs := &ControlService{
		slogger:         k.Slogger().With("component", "control"),
		knapsack:        k,
		requestInterval: atomic.NewDuration(k.ControlRequestInterval()),
		fetcher:         fetcher,
		lastFetched:     make(map[string]string),
		consumers:       make(map[string]consumer),
		subscribers:     make(map[string][]subscriber),
	}

	for _, opt := range opts {
		opt(cs)
	}

	cs.requestTicker = time.NewTicker(cs.requestInterval.Load())

	// Observe ControlRequestInterval changes to know when to accelerate/decelerate fetching frequency
	cs.knapsack.RegisterChangeObserver(cs, keys.ControlRequestInterval)

	return cs
}

// ExecuteWithContext returns an Execute function suitable for rungroup. It's a
// wrapper over the Start function, which takes a context.Context.
func (cs *ControlService) ExecuteWithContext(ctx context.Context) func() error {
	return func() error {
		ctx, cs.cancel = context.WithCancel(ctx)
		cs.Start(ctx)
		return nil
	}
}

// Start is the main control service loop. It fetches control
func (cs *ControlService) Start(ctx context.Context) {
	cs.slogger.Log(ctx, slog.LevelInfo,
		"control service started",
	)

	startUpMessageSuccess := false

	for {
		fetchErr := cs.Fetch(context.TODO())
		switch {
		case fetchErr != nil:
			cs.slogger.Log(ctx, slog.LevelWarn,
				"failed to fetch data from control server. Not fatal, moving on",
				"err", fetchErr,
			)
		case !startUpMessageSuccess:
			if err := cs.SendMessage("startup", cs.startupData(ctx)); err != nil {
				cs.slogger.Log(ctx, slog.LevelWarn,
					"failed to send startup message on control server start",
					"err", err,
				)
				break
			}

			startUpMessageSuccess = true
		}

		select {
		case <-ctx.Done():
			return
		case <-cs.requestTicker.C:
			// Go fetch!
			continue
		}
	}
}

// startupData retrieves data to be reported to the control server on launcher startup.
func (cs *ControlService) startupData(ctx context.Context) map[string]string {
	data := map[string]string{
		"launcher_version": version.Version().Version,
		"run_id":           cs.knapsack.GetRunID(),
	}

	if enrollStatus, err := cs.knapsack.CurrentEnrollmentStatus(); err != nil {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"could not get enrollment status for startup message",
			"err", err,
		)
	} else {
		data["current_enrollment_status"] = string(enrollStatus)
	}

	if deviceId, err := cs.knapsack.ServerProvidedDataStore().Get([]byte("device_id")); err != nil {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"could not get device ID for startup message",
			"err", err,
		)
	} else {
		data["device_id"] = string(deviceId)
	}

	if munemo, err := cs.knapsack.ServerProvidedDataStore().Get([]byte("munemo")); err != nil {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"could not get munemo for startup message",
			"err", err,
		)
	} else {
		data["munemo"] = string(munemo)
	}

	if orgId, err := cs.knapsack.ServerProvidedDataStore().Get([]byte("organization_id")); err != nil {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"could not get organization ID for startup message",
			"err", err,
		)
	} else {
		data["organization_id"] = string(orgId)
	}

	// Get serial number from enrollment details
	enrollmentDetails := cs.knapsack.GetEnrollmentDetails()
	if enrollmentDetails.HardwareSerial != "" {
		data["serial_number"] = enrollmentDetails.HardwareSerial
	} else {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"could not get serial number from enrollment details",
		)
	}

	return data
}

func (cs *ControlService) Interrupt(_ error) {
	cs.Stop()
}

func (cs *ControlService) Stop() {
	cs.slogger.Log(context.TODO(), slog.LevelInfo,
		"control service stopping",
	)
	cs.requestTicker.Stop()
	if cs.cancel != nil {
		cs.cancel()
	}
}

func (cs *ControlService) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	if slices.Contains(flagKeys, keys.ControlRequestInterval) {
		cs.requestIntervalChanged(ctx, cs.knapsack.ControlRequestInterval())
	}
}

func (cs *ControlService) requestIntervalChanged(ctx context.Context, newInterval time.Duration) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	currentRequestInterval := cs.requestInterval.Load()
	if newInterval == currentRequestInterval {
		return
	}

	// Perform a fetch now, to retrieve data faster in case this change
	// was triggered by localserver or the user clicking on the menu bar app
	// instead of by a control server change.
	if err := cs.Fetch(ctx); err != nil {
		// if we got an error, log it and move on
		cs.slogger.Log(ctx, slog.LevelWarn,
			"failed to fetch data from control server. Not fatal, moving on",
			"err", err,
		)
	}

	if newInterval < currentRequestInterval {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"accelerating control service request interval",
			"new_interval", newInterval.String(),
			"old_interval", currentRequestInterval.String(),
		)
	} else {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"resetting control service request interval after acceleration",
			"new_interval", newInterval.String(),
			"old_interval", currentRequestInterval.String(),
		)
	}

	// restart the ticker on new interval
	cs.requestInterval.Store(newInterval)
	cs.requestTicker.Reset(newInterval)
}

// Performs a retrieval of the latest control server data, and notifies observers of updates.
func (cs *ControlService) Fetch(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// Do not block in the case where:
	// 1. `Start` called `Fetch` on an interval
	// 2. The control service received an updated `ControlRequestInterval` from the server
	// 3. `requestIntervalChanged` is now attempting to call `Fetch` again before updating the interval
	// We do want to allow `requestIntervalChanged` to call `Fetch` to handle the case where
	// `ControlRequestInterval` updated via a different mechanism, like a request to localserver
	// or the user clicking on the menu bar app.
	if !cs.fetchMutex.TryLock() {
		return errors.New("fetch is currently executing elsewhere")
	}
	span.AddEvent("fetch_lock_acquired")
	defer func() {
		cs.fetchMutex.Unlock()
		span.AddEvent("fetch_lock_released")
	}()

	// Empty hash means get the map of subsystems & hashes
	data, err := cs.fetcher.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("getting subsystems map: %w", err)
	}

	if data == nil {
		return errors.New("subsystems map data is nil")
	}

	var subsystems map[string]string
	if err := json.NewDecoder(data).Decode(&subsystems); err != nil {
		return fmt.Errorf("decoding subsystems map: %w", err)
	}

	fetchFull := false
	cs.fetchFullMutex.Lock()
	span.AddEvent("fetch_full_lock_acquired")
	if cs.fetchFull {
		fetchFull = true     // fetch all subsystems during this Fetch
		cs.fetchFull = false // reset for next Fetch
	}
	cs.fetchFullMutex.Unlock()
	span.AddEvent("fetch_full_lock_released")

	for subsystem, hash := range subsystems {
		if !cs.knownSubsystem(subsystem) {
			// Ignore unknown subsystems.
			continue
		}

		lastHash, ok := cs.lastFetched[subsystem]
		if !ok && cs.store != nil {
			// Try to get the stored hash. If we can't get it, no worries, it means we don't have a last hash value,
			// and we can just move on.
			if storedHash, err := cs.store.Get([]byte(subsystem)); err == nil {
				lastHash = string(storedHash)
			}
		}

		if hash == lastHash && !cs.knapsack.ForceControlSubsystems() && !fetchFull {
			// The last fetched update is still fresh
			// Nothing to do, skip to the next subsystem
			continue
		}

		if err := cs.fetchAndUpdate(ctx, subsystem, hash); err != nil {
			cs.slogger.Log(ctx, slog.LevelDebug,
				"failed to fetch object. skipping...",
				"subsystem", subsystem,
				"err", err,
			)
			continue
		}
	}

	return nil
}

// Fetches latest subsystem data, and notifies observers of updates.
func (cs *ControlService) fetchAndUpdate(ctx context.Context, subsystem, hash string) error {
	ctx, span := observability.StartSpan(ctx, "subsystem", subsystem)
	defer span.End()

	slogger := cs.slogger.With("subsystem", subsystem)

	data, err := cs.fetcher.GetSubsystemData(ctx, hash)
	if err != nil {
		return fmt.Errorf("failed to get control data: %w", err)
	}

	if data == nil {
		return errors.New("control data is nil")
	}

	// Consumer and subscriber(s) notified now
	if err := cs.update(ctx, subsystem, data); err != nil {
		// Returning the error so we don't store the hash and we can try again next time
		slogger.Log(ctx, slog.LevelWarn,
			"failed to update consumers and subscribers",
			"err", err,
		)
		return err
	}

	// Remember the hash of the last fetched version of this subsystem's data
	cs.lastFetched[subsystem] = hash

	// can't store hash if we dont have store
	if cs.store == nil {
		return nil
	}

	// Store the hash so we can persist the last fetched data across launcher restarts
	if err := cs.store.Set([]byte(subsystem), []byte(hash)); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"failed to store last fetched control data",
			"err", err,
		)
	}

	return nil
}

// knownSubsystem checks our registered consumers and subscribers to see if the given
// subsystem is one that has been registered with the control service.
func (cs *ControlService) knownSubsystem(subsystem string) bool {
	if _, subsystemFromConsumer := cs.consumers[subsystem]; subsystemFromConsumer {
		return true
	}
	if _, subsystemFromSubscriber := cs.subscribers[subsystem]; subsystemFromSubscriber {
		return true
	}

	return false
}

// Registers a consumer for ingesting subsystem updates
func (cs *ControlService) RegisterConsumer(subsystem string, consumer consumer) error {
	if _, ok := cs.consumers[subsystem]; ok {
		return fmt.Errorf("subsystem %s already has registered consumer", subsystem)
	}
	cs.consumers[subsystem] = consumer
	return nil
}

// Registers a subscriber for subsystem update notifications
func (cs *ControlService) RegisterSubscriber(subsystem string, subscriber subscriber) {
	cs.subscribers[subsystem] = append(cs.subscribers[subsystem], subscriber)
}

func (cs *ControlService) SendMessage(method string, params interface{}) error {
	return cs.fetcher.SendMessage(context.TODO(), method, params)
}

// Updates all registered consumers and subscribers of subsystem updates
func (cs *ControlService) update(ctx context.Context, subsystem string, reader io.Reader) error {
	_, span := observability.StartSpan(ctx, "subsystem", subsystem)
	defer span.End()

	// First, send to consumer, if any
	if consumer, ok := cs.consumers[subsystem]; ok {
		if err := consumer.Update(reader); err != nil {
			// No need to ping subscribers if the consumer update failed
			return err
		}
	}

	// Then send a ping to all subscribers
	for _, subscriber := range cs.subscribers[subsystem] {
		subscriber.Ping()
	}

	return nil
}

// Do handles the force_full_control_data_fetch action.
func (cs *ControlService) Do(data io.Reader) error {
	ctx, span := observability.StartSpan(context.TODO(), "action", ForceFullControlDataFetchAction)
	defer span.End()

	cs.slogger.Log(ctx, slog.LevelDebug,
		"received request to perform full fetch of all subsystems",
	)

	// We receive this request in the middle of a `Fetch`, so we can't call
	// `Fetch` again immediately. Instead, set `fetchFull` so that the next
	// call to `Fetch` will fetch all subsystems.
	span.AddEvent("fetch_full_lock_acquired")
	cs.fetchFullMutex.Lock()

	defer func() {
		cs.fetchFullMutex.Unlock()
		span.AddEvent("fetch_full_lock_released")
	}()
	cs.fetchFull = true

	// Treat this action as best-effort: try to do it once, no need to retry.
	return nil
}

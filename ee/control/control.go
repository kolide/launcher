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
	"golang.org/x/exp/slices"
)

// ControlService is the main object that manages the control service. It is responsible for fetching
// and caching control data, and updating consumers and subscribers.
type ControlService struct {
	slogger              *slog.Logger
	knapsack             types.Knapsack
	cancel               context.CancelFunc
	requestIntervalMutex *sync.RWMutex
	requestInterval      time.Duration
	requestTicker        *time.Ticker
	fetcher              dataProvider
	fetchMutex           sync.Mutex
	store                types.GetterSetter
	lastFetched          map[string]string
	consumers            map[string]consumer
	subscribers          map[string][]subscriber
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
	GetConfig() (io.Reader, error)
	GetSubsystemData(hash string) (io.Reader, error)
	SendMessage(method string, params interface{}) error
}

func New(k types.Knapsack, fetcher dataProvider, opts ...Option) *ControlService {
	cs := &ControlService{
		slogger:              k.Slogger().With("component", "control"),
		knapsack:             k,
		requestInterval:      k.ControlRequestInterval(),
		requestIntervalMutex: &sync.RWMutex{},
		fetcher:              fetcher,
		lastFetched:          make(map[string]string),
		consumers:            make(map[string]consumer),
		subscribers:          make(map[string][]subscriber),
	}

	for _, opt := range opts {
		opt(cs)
	}

	cs.requestTicker = time.NewTicker(cs.requestInterval)

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
		fetchErr := cs.Fetch()
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

	if serialNumber, err := cs.knapsack.ServerProvidedDataStore().Get([]byte("serial_number")); err != nil {
		cs.slogger.Log(ctx, slog.LevelDebug,
			"could not get serial number for startup message",
			"err", err,
		)
	} else {
		data["serial_number"] = string(serialNumber)
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

func (cs *ControlService) FlagsChanged(flagKeys ...keys.FlagKey) {
	if slices.Contains(flagKeys, keys.ControlRequestInterval) {
		cs.requestIntervalChanged(cs.knapsack.ControlRequestInterval())
	}
}

func (cs *ControlService) requestIntervalChanged(newInterval time.Duration) {
	currentRequestInterval := cs.readRequestInterval()
	if newInterval == currentRequestInterval {
		return
	}

	// Perform a fetch now, to retrieve data faster in case this change
	// was triggered by localserver or the user clicking on the menu bar app
	// instead of by a control server change.
	if err := cs.Fetch(); err != nil {
		// if we got an error, log it and move on
		cs.slogger.Log(context.TODO(), slog.LevelWarn,
			"failed to fetch data from control server. Not fatal, moving on",
			"err", err,
		)
	}

	if newInterval < currentRequestInterval {
		cs.slogger.Log(context.TODO(), slog.LevelDebug,
			"accelerating control service request interval",
			"new_interval", newInterval.String(),
			"old_interval", currentRequestInterval.String(),
		)
	} else {
		cs.slogger.Log(context.TODO(), slog.LevelDebug,
			"resetting control service request interval after acceleration",
			"new_interval", newInterval.String(),
			"old_interval", currentRequestInterval.String(),
		)
	}

	// restart the ticker on new interval
	cs.setRequestInterval(newInterval)
	cs.requestTicker.Reset(newInterval)
}

func (cs *ControlService) setRequestInterval(interval time.Duration) {
	cs.requestIntervalMutex.Lock()
	defer cs.requestIntervalMutex.Unlock()
	cs.requestInterval = interval
}

func (cs *ControlService) readRequestInterval() time.Duration {
	cs.requestIntervalMutex.RLock()
	defer cs.requestIntervalMutex.RUnlock()
	return cs.requestInterval
}

// Performs a retrieval of the latest control server data, and notifies observers of updates.
func (cs *ControlService) Fetch() error {
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
	defer cs.fetchMutex.Unlock()

	// Empty hash means get the map of subsystems & hashes
	data, err := cs.fetcher.GetConfig()
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

	for subsystem, hash := range subsystems {
		lastHash, ok := cs.lastFetched[subsystem]
		if !ok && cs.store != nil {
			// Try to get the stored hash. If we can't get it, no worries, it means we don't have a last hash value,
			// and we can just move on.
			if storedHash, err := cs.store.Get([]byte(subsystem)); err == nil {
				lastHash = string(storedHash)
			}
		}

		if hash == lastHash && !cs.knapsack.ForceControlSubsystems() {
			// The last fetched update is still fresh
			// Nothing to do, skip to the next subsystem
			continue
		}

		if err := cs.fetchAndUpdate(subsystem, hash); err != nil {
			cs.slogger.Log(context.TODO(), slog.LevelDebug,
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
func (cs *ControlService) fetchAndUpdate(subsystem, hash string) error {
	slogger := cs.slogger.With("subsystem", subsystem)
	data, err := cs.fetcher.GetSubsystemData(hash)
	if err != nil {
		return fmt.Errorf("failed to get control data: %w", err)
	}

	if data == nil {
		return errors.New("control data is nil")
	}

	// Consumer and subscriber(s) notified now
	if err := cs.update(subsystem, data); err != nil {
		// Although we failed to update, the payload may be bad and there's no
		// sense in repeatedly attempting to apply a bad update.
		// A new update will have a new hash, so continue and remember this hash.
		slogger.Log(context.TODO(), slog.LevelError,
			"failed to update consumers and subscribers",
			"err", err,
		)
	}

	// Remember the hash of the last fetched version of this subsystem's data
	cs.lastFetched[subsystem] = hash

	// can't store hash if we dont have store
	if cs.store == nil {
		return nil
	}

	// Store the hash so we can persist the last fetched data across launcher restarts
	if err := cs.store.Set([]byte(subsystem), []byte(hash)); err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"failed to store last fetched control data",
			"err", err,
		)
	}

	return nil
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
	return cs.fetcher.SendMessage(method, params)
}

// Updates all registered consumers and subscribers of subsystem updates
func (cs *ControlService) update(subsystem string, reader io.Reader) error {
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

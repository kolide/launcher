package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/types"
)

// ControlService is the main object that manages the control service. It is responsible for fetching
// and caching control data, and updating consumers and subscribers.
type ControlService struct {
	logger                   log.Logger
	knapsack                 *knapsack.Knapsack
	cancel                   context.CancelFunc
	requestInterval          time.Duration
	requestTicker            *time.Ticker
	requestAccelerationTimer *time.Timer
	minAccelerationInterval  time.Duration
	fetcher                  dataProvider
	fetchMutex               sync.Mutex
	store                    types.GetterSetter
	lastFetched              map[string]string
	consumers                map[string]consumer
	subscribers              map[string][]subscriber
}

// consumer is an interface for something that consumes control server data updates. The
// control server supports at most one consumer per subsystem.
type consumer interface {
	Update(io.Reader) error
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
}

func New(logger log.Logger, k *knapsack.Knapsack, fetcher dataProvider, opts ...Option) *ControlService {
	cs := &ControlService{
		logger:                  log.With(logger, "component", "control"),
		knapsack:                k,
		requestInterval:         60 * time.Second,
		minAccelerationInterval: 5 * time.Second,
		fetcher:                 fetcher,
		lastFetched:             make(map[string]string),
		consumers:               make(map[string]consumer),
		subscribers:             make(map[string][]subscriber),
	}

	cs.requestTicker = time.NewTicker(cs.requestInterval)

	for _, opt := range opts {
		opt(cs)
	}

	return cs
}

// ExecuteWithContext returns an Execute function suitable for rungroup. It's a
// wrapper over the Start function, which takes a context.Context.
func (cs *ControlService) ExecuteWithContext(ctx context.Context) func() error {
	return func() error {
		cs.Start(ctx)
		return nil
	}
}

// Start is the main control service loop. It fetches control
func (cs *ControlService) Start(ctx context.Context) {
	level.Info(cs.logger).Log("msg", "control service started")
	ctx, cs.cancel = context.WithCancel(ctx)
	for {
		// Fetch immediately on each iteration, avoiding the initial ticker delay
		if err := cs.Fetch(); err != nil {
			level.Debug(cs.logger).Log(
				"msg", "failed to fetch data from control server. Not fatal, moving on",
				"err", err,
			)
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

func (cs *ControlService) Interrupt(err error) {
	level.Info(cs.logger).Log("msg", "control service interrupted", "err", err)
	cs.Stop()
}

func (cs *ControlService) Stop() {
	level.Info(cs.logger).Log("msg", "control service stopping")
	if cs.cancel != nil {
		cs.cancel()
	}
}

func (cs *ControlService) AccelerateRequestInterval(interval, duration time.Duration) {
	// perform a fetch now
	if err := cs.Fetch(); err != nil {
		// if we got an error, log it and move on
		level.Debug(cs.logger).Log(
			"msg", "failed to fetch data from control server. Not fatal, moving on",
			"err", err,
		)
	}

	if interval < cs.minAccelerationInterval {
		level.Debug(cs.logger).Log(
			"msg", "control service acceleration interval too small, using minimum interval",
			"minimum_interval", cs.minAccelerationInterval,
			"provided_interval", interval,
		)
		interval = cs.minAccelerationInterval
	}

	// stop existing timer
	if cs.requestAccelerationTimer != nil {
		cs.requestAccelerationTimer.Stop()
	}

	// set request interval back to starting interval after duration has passed
	cs.requestAccelerationTimer = time.AfterFunc(duration, func() {
		level.Debug(cs.logger).Log(
			"msg", "resetting control service request interval after acceleration",
			"interval", cs.requestInterval,
		)

		// set back to original interval
		cs.requestTicker.Reset(cs.requestInterval)
	})

	level.Debug(cs.logger).Log(
		"msg", "accelerating control service request interval",
		"interval", interval,
		"duration", duration,
	)

	// restart the ticker on accelerated interval
	cs.requestTicker.Reset(interval)
}

// Performs a retrieval of the latest control server data, and notifies observers of updates.
func (cs *ControlService) Fetch() error {
	cs.fetchMutex.Lock()
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
		logger := log.With(cs.logger, "subsystem", subsystem)
		lastHash, ok := cs.lastFetched[subsystem]
		if !ok && cs.store != nil {
			// Try to get the stored hash. If we can't get it, no worries, it means we don't have a last hash value,
			// and we can just move on.
			if storedHash, err := cs.store.Get([]byte(subsystem)); err == nil {
				lastHash = string(storedHash)
			}
		}

		if hash == lastHash && !cs.knapsack.Flags.ForceControlSubsystems() {
			// The last fetched update is still fresh
			// Nothing to do, skip to the next subsystem
			continue
		}

		if err := cs.fetchAndUpdate(subsystem, hash); err != nil {
			level.Debug(logger).Log("msg", "failed to fetch object. skipping...", "err", err)
			continue
		}
	}

	level.Debug(cs.logger).Log("msg", "control data fetch complete")

	return nil
}

// Fetches latest subsystem data, and notifies observers of updates.
func (cs *ControlService) fetchAndUpdate(subsystem, hash string) error {
	logger := log.With(cs.logger, "subsystem", subsystem)
	data, err := cs.fetcher.GetSubsystemData(hash)
	if err != nil {
		return fmt.Errorf("failed to get control data: %w", err)
	}

	if data == nil {
		return errors.New("control data is nil")
	}

	// Consumer and subscriber(s) notified now
	err = cs.update(subsystem, data)
	if err != nil {
		// Although we failed to update, the payload may be bad and there's no
		// sense in repeatedly attempting to apply a bad update.
		// A new update will have a new hash, so continue and remember this hash.
		level.Debug(logger).Log("msg", "failed to update consumers and subscribers", "err", err)
	}

	// Remember the hash of the last fetched version of this subsystem's data
	cs.lastFetched[subsystem] = hash

	if cs.store != nil {
		// Store the hash so we can persist the last fetched data across launcher restarts
		err = cs.store.Set([]byte(subsystem), []byte(hash))
		if err != nil {
			level.Error(logger).Log("msg", "failed to store last fetched control data", "err", err)
		}
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

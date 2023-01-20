package control

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent"
)

// ControlService is the main object that manages the control service. It is responsible for fetching
// and caching control data, and updating consumers and subscribers.
type ControlService struct {
	logger          log.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	requestInterval time.Duration
	fetcher         dataProvider
	storer          agent.RetrieverStorer
	lastFetched     map[string]string
	consumers       map[string]consumer
	subscribers     map[string][]subscriber
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
	Get(hash string) (data io.Reader, err error)
}

func New(logger log.Logger, ctx context.Context, fetcher dataProvider, opts ...Option) *ControlService {
	cs := &ControlService{
		logger:          logger,
		ctx:             ctx,
		requestInterval: 60 * time.Second,
		fetcher:         fetcher,
		lastFetched:     make(map[string]string),
		consumers:       make(map[string]consumer),
		subscribers:     make(map[string][]subscriber),
	}

	for _, opt := range opts {
		opt(cs)
	}

	return cs
}

func (cs *ControlService) Execute() error {
	level.Info(cs.logger).Log("msg", "control service started")
	cs.Start(cs.ctx)
	return nil
}

func (cs *ControlService) Start(ctx context.Context) {
	ctx, cs.cancel = context.WithCancel(ctx)
	requestTicker := time.NewTicker(cs.requestInterval)
	for {
		// Fetch immediately on each iteration, avoiding the initial ticker delay
		if err := cs.Fetch(); err != nil {
			level.Debug(cs.logger).Log(
				"msg", "failed to fetch data from control server",
				"err", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-requestTicker.C:
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
	cs.cancel()
}

// Performs a retrieval of the latest control server data, and notifies observers of updates.
func (cs *ControlService) Fetch() error {
	// Empty hash means get the map of subsystems & hashes
	data, err := cs.fetcher.Get("")
	if err != nil {
		return fmt.Errorf("getting subsystems map: %w", err)
	}

	if data == nil {
		return fmt.Errorf("subsystems map data is nil")
	}

	var subsystems map[string]string
	if err := json.NewDecoder(data).Decode(&subsystems); err != nil {
		return fmt.Errorf("decoding subsystems map: %w", err)
	}

	for subsystem, hash := range subsystems {
		lastHash, ok := cs.lastFetched[subsystem]
		if !ok && cs.storer != nil {
			storedHash, err := cs.storer.Get([]byte(subsystem))
			if err != nil {
				level.Debug(cs.logger).Log(
					"msg", "failed to get last fetched hash from stored data",
					"subsystem", subsystem,
					"err", err)
				// This isn't a fatal error
			} else {
				lastHash = string(storedHash)
			}
		}

		if hash == lastHash {
			// The last fetched update is still fresh
			// Nothing to do, skip to the next subsystem
			continue
		}

		if err = cs.fetchAndUpdate(subsystem, hash); err != nil {
			return err
		}
	}

	level.Debug(cs.logger).Log("msg", "control data fetch complete")

	return nil
}

// Fetches latest subsystem data, and notifies observers of updates.
func (cs *ControlService) fetchAndUpdate(subsystem, hash string) error {
	data, err := cs.fetcher.Get(hash)
	if err != nil {
		return fmt.Errorf("failed to get control data: %w", err)
	}

	if data == nil {
		return fmt.Errorf("control data is nil")
	}

	// Consumer and subscriber(s) notified now
	err = cs.update(subsystem, data)
	if err != nil {
		level.Error(cs.logger).Log(
			"msg", "failed to update consumers and subscribers",
			"subsystem", subsystem,
			"err", err)
		// Although we failed to update, the payload may be bad and there's no
		// sense in repeatedly attempting to apply a bad update.
		// A new update will have a new hash, so continue and remember this hash.
	}

	// Remember the hash of the last fetched version of this subsystem's data
	cs.lastFetched[subsystem] = hash

	if cs.storer != nil {
		// Store the hash so we can persist the last fetched data across launcher restarts
		err = cs.storer.Set([]byte(subsystem), []byte(hash))
		if err != nil {
			level.Error(cs.logger).Log(
				"msg", "failed to store last fetched control data",
				"subsystem", subsystem,
				"err", err)

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

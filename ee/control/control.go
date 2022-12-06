package control

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// Registry is a registrar of consumers & subscribers
type Registry struct {
	consumers   map[string]consumer
	subscribers map[string][]subscriber
}

// ControlService is the main object that manages the control service. It is responsible for fetching
// and caching control data, and updating consumers and subscribers.
type ControlService struct {
	Registry
	logger          log.Logger
	cancel          context.CancelFunc
	requestInterval time.Duration
	fetcher         dataProvider
	lastFetched     map[string]string
}

// consumer is an interface for something that consumes control server data updates. The
// control server supports at most one consumer per subsystem.
type consumer interface {
	Update(io.Reader)
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

func New(logger log.Logger, fetcher dataProvider, opts ...Option) *ControlService {
	r := Registry{
		consumers:   make(map[string]consumer),
		subscribers: make(map[string][]subscriber),
	}
	cs := &ControlService{
		Registry:        r,
		logger:          logger,
		requestInterval: 60 * time.Second,
		fetcher:         fetcher,
		lastFetched:     make(map[string]string),
	}

	for _, opt := range opts {
		opt(cs)
	}

	return cs
}

func (cs *ControlService) Start(ctx context.Context) {
	ctx, cs.cancel = context.WithCancel(ctx)
	requestTicker := time.NewTicker(cs.requestInterval)
	for {
		// Fetch immediately on each iteration, avoiding the initial ticker delay
		cs.Fetch()

		select {
		case <-ctx.Done():
			return
		case <-requestTicker.C:
			// Go fetch!
			continue
		}
	}
}

func (cs *ControlService) Stop() {
	cs.cancel()
}

// Performs a retrieval of the latest control server data, and notifies observers of updates.
func (cs *ControlService) Fetch() error {
	// Empty hash means get the subsystem map of hashes
	data, err := cs.fetcher.Get("")
	if err != nil {
		return fmt.Errorf("getting subsystems map: %w", err)
	}

	var subsystems map[string]string
	if err := json.NewDecoder(data).Decode(&subsystems); err != nil {
		return fmt.Errorf("decoding subsystems map: %w", err)
	}

	for subsystem, hash := range subsystems {
		if hash == cs.lastFetched[subsystem] {
			// The most recent hash matches the last one we fetched
			// Nothing to do, skip to the next subsystem
			continue
		}

		data, err := cs.fetcher.Get(hash)
		if err != nil {
			return fmt.Errorf("failed to get control data: %w", err)
		}

		// Consumer and subscriber(s) notified now
		cs.update(subsystem, data)

		// Remember the hash of the last fetched version of this subsystem's data
		cs.lastFetched[subsystem] = hash
	}

	level.Debug(cs.logger).Log("msg", "control data fetch complete")

	return nil
}

func (r *Registry) RegisterConsumer(subsystem string, consumer consumer) error {
	if _, ok := r.consumers[subsystem]; ok {
		return fmt.Errorf("subsystem %s already has registered consumer", subsystem)
	}
	r.consumers[subsystem] = consumer
	return nil
}

func (r *Registry) RegisterSubscriber(subsystem string, subscriber subscriber) {
	r.subscribers[subsystem] = append(r.subscribers[subsystem], subscriber)
}

func (r *Registry) update(subsystem string, reader io.Reader) {
	// First, send to consumer, if any
	if consumer, ok := r.consumers[subsystem]; ok {
		consumer.Update(reader)
	}

	// Then send a ping to all subscribers
	for _, subscriber := range r.subscribers[subsystem] {
		subscriber.Ping()
	}
}

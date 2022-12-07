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

// ControlService is the main object that manages the control service. It is responsible for fetching
// and caching control data, and updating consumers and subscribers.
type ControlService struct {
	logger          log.Logger
	cancel          context.CancelFunc
	requestInterval time.Duration
	consumers       map[string]consumer
	subscribers     map[string][]subscriber
	data            dataProvider
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
	Get(subsystem string) (hash string, data io.Reader, err error)
}

func New(logger log.Logger, data dataProvider, opts ...Option) *ControlService {
	cs := &ControlService{
		logger:          logger,
		requestInterval: 60 * time.Second,
		consumers:       make(map[string]consumer),
		subscribers:     make(map[string][]subscriber),
		data:            data,
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
		select {
		case <-ctx.Done():
			return
		case <-requestTicker.C:
			cs.Fetch()
		}
	}
}

func (cs *ControlService) Stop() {
	cs.cancel()
}

// controlResponse is the payload received from the control server
type controlResponse struct {
	// TODO: This is a temporary and simple data format for phase 1
	Message string `json:"message,omitempty"`
	Err     string `json:"error,omitempty"`
}

// Performs a retrieval of the latest control server data, and notifies observers of updates.
func (cs *ControlService) Fetch() error {
	_, data, err := cs.data.Get("")
	if err != nil {
		return fmt.Errorf("getting initial config: %w", err)
	}

	var controlData controlResponse
	if err := json.NewDecoder(data).Decode(&controlData); err != nil {
		return fmt.Errorf("decoding initial config map: %w", err)
	}

	level.Debug(cs.logger).Log(
		"msg", "Got response",
		"controlData.Message", controlData.Message,
	)

	// TODO: Here's where the subsystems, and their consumers and subscribers come into play

	// Here's a pseudo code outline of what would follow here

	// for each (subsystem, hash) in the list
	// 	if the hash matches the last update for this subsystem
	// 		skip to next subsystem
	// 	otherwise
	// 		ask the dataProvider for the latest update for this subsystem (make a new HTTP request)

	// 	if latest update is successfully fetched
	// 		notify the consumer of this subsystem, and give them the control data

	// 		for each subscriber of this subsystem
	// 			ping the subscriber

	// 		cache latest update hash retrieved for this subsystem

	return nil
}

func (cs *ControlService) RegisterConsumer(subsystem string, consumer consumer) error {
	if _, ok := cs.consumers[subsystem]; ok {
		return fmt.Errorf("subsystem %s already has registered consumer", subsystem)
	}
	cs.consumers[subsystem] = consumer
	return nil
}

func (cs *ControlService) RegisterSubscriber(subsystem string, subscriber subscriber) {
	cs.subscribers[subsystem] = append(cs.subscribers[subsystem], subscriber)
}

func (cs *ControlService) update(subsystem string, reader io.Reader) {
	// First, send to consumer, if any
	if consumer, ok := cs.consumers[subsystem]; ok {
		consumer.Update(reader)
	}

	// Then send a ping to all subscribers
	for _, subscriber := range cs.subscribers[subsystem] {
		subscriber.Ping()
	}
}

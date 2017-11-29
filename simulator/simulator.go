package simulator

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// QueryRunner is the interface which defines the pluggable behavior of a simulated
// host. Each "type" of host may have their own implementation of this interface.
type QueryRunner interface {
	RunQuery(sql string) (results []map[string]string, err error)
}

// HostSimulation is the type which contains the state of a simulated host
type HostSimulation struct {
	// the following define the configurable aspects of the simulation
	host                   QueryRunner
	insecure               bool
	insecureGrpc           bool
	requestQueriesInterval time.Duration
	requestConfigInterval  time.Duration
	publishResultsInterval time.Duration

	// The state of the simulation is gated with a read/write lock.
	// To read something in state:
	//
	//   h.state.lock.RLock()
	//   defer h.state.lock.RUnlock()
	//
	// To write state based on the on-going simulation:
	//
	//   h.state.lock.Lock()
	//   defer h.state.lock.Unlock()
	state *hostSimulationState

	shutdown chan bool
	done     chan bool
}

// hostSimulationState is a light container around simulation state management
type hostSimulationState struct {
	lock    sync.RWMutex
	failed  bool
	started bool
}

// SimulationOption is a functional option pattern for defining how a host
// simulation instance should be configured. For more information on this
// patter, see the following blog post:
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type SimulationOption func(*HostSimulation)

// WithQueryRunner is a functional option which allows the user to declare the
// behavior of the simulated host
func WithQueryRunner(host QueryRunner) SimulationOption {
	return func(i *HostSimulation) {
		i.host = host
	}
}

// WithRequestQueriesInterval is a functional option which allows the user to
// specify how often the simulation host should check-in to the server and ask
// for queries to run
func WithRequestQueriesInterval(interval time.Duration) SimulationOption {
	return func(i *HostSimulation) {
		i.requestQueriesInterval = interval
	}
}

// WithRequestConfigInterval is a functional option which allows the user to
// specify how often the simulation host should check-in to the server and ask
// for a new config
func WithRequestConfigInterval(interval time.Duration) SimulationOption {
	return func(i *HostSimulation) {
		i.requestConfigInterval = interval
	}
}

// WithPublishResultsInterval is a functional option which allows the user to
// specify how often the simulation host should log status and results logs
func WithPublishResultsInterval(interval time.Duration) SimulationOption {
	return func(i *HostSimulation) {
		i.publishResultsInterval = interval
	}
}

// WithInsecure is a functional option which allows the user to declare that the
// remote API should be connected to over an insecure channel
func WithInsecure() SimulationOption {
	return func(i *HostSimulation) {
		i.insecure = true
	}
}

// WithInsecureGrpc is a functional option which allows the user to declare that
// the remote API should be connected to over an insecure gRPC channel
func WithInsecureGrpc() SimulationOption {
	return func(i *HostSimulation) {
		i.insecureGrpc = true
	}
}

// createSimulationRuntime is an internal helper which creates an instance of
// *HostSimulation given a set of supplied functional options
func createSimulationRuntime(opts ...SimulationOption) *HostSimulation {
	h := &HostSimulation{
		requestQueriesInterval: 2 * time.Second,
		requestConfigInterval:  5 * time.Second,
		publishResultsInterval: 10 * time.Second,
		shutdown:               make(chan bool),
		done:                   make(chan bool),
		state:                  &hostSimulationState{},
	}
	for _, opt := range opts {
		opt(h)
	}

	return h
}

// LaunchSimulation is a utility which allows the user to configure and run an
// asynchronous instance of a host simulation given a set of functional options
func LaunchSimulation(opts ...SimulationOption) *HostSimulation {
	h := createSimulationRuntime(opts...)
	go func() {
		h.state.lock.Lock()
		h.state.started = true
		h.state.lock.Unlock()

		requestQueriesTicker := time.NewTicker(h.requestQueriesInterval)
		requestConfigTicker := time.NewTicker(h.requestConfigInterval)
		publishResultsTicker := time.NewTicker(h.publishResultsInterval)

		for {
			select {
			case <-requestQueriesTicker.C:
				fmt.Println("requestQueries")
			case <-requestConfigTicker.C:
				fmt.Println("requestConfig")
			case <-publishResultsTicker.C:
				fmt.Println("publishResults")
			case <-h.shutdown:
				h.done <- true
				return
			}
		}

	}()
	return h
}

// Healthy is a helper which performs an introspection on the simulation
// instance to determine whether or not it is healthy
func (h *HostSimulation) Healthy() bool {
	// we're going to be reading the state of the instance, so we first must
	// acquire a read lock on the state
	h.state.lock.RLock()
	defer h.state.lock.RUnlock()

	if h.state.started {
		return !h.state.failed
	}
	return true
}

func (h *HostSimulation) Shutdown() error {
	h.shutdown <- true

	timer := time.NewTimer(time.Second)
	select {
	case <-h.done:
		return nil
	case <-timer.C:
	}

	return errors.New("simulation did not shut down in time")
}

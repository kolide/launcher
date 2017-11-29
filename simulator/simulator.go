package simulator

import "errors"

// FakeHost is the interface which defines the pluggable behavior of a simulated
// host. Each "type" of host may have their own implementation of this interface.
type FakeHost interface {
	RunQuery(sql string) (results []map[string]string, err error)
}

// HostSimulation is the type which contains the state of a simulated host
type HostSimulation struct {
	// the following define the configurable aspects of the simulation
	host         FakeHost
	insecure     bool
	insecureGrpc bool

	// the following define the operating state of the simulation
	failed  bool
	started bool
}

// SimulationOption is a functional option pattern for defining how a host
// simulation instance should be configured. For more information on this
// patter, see the following blog post:
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type SimulationOption func(*HostSimulation)

// WithHost is a functional option which allows the user to declare the behavior
// of the simulated host
func WithHost(host FakeHost) SimulationOption {
	return func(i *HostSimulation) {
		i.host = host
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
	h := &HostSimulation{}
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
		if err := h.run(); err != nil {
			h.failed = true
		}
	}()
	return h
}

// Healthy is a helper which performs an introspection on the simulation
// instance to determine whether or not it is healthy
func (h *HostSimulation) Healthy() bool {
	if h.started {
		return !h.failed
	}
	return true
}

// run launches the simulation synchronously
func (h *HostSimulation) run() error {
	h.started = true
	return errors.New("unimplemented")
}

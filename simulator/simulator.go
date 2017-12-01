package simulator

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
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
	uuid                   string
	enrollSecret           string
	grpcURL                string
	insecure               bool
	insecureGrpc           bool
	requestQueriesInterval time.Duration
	requestConfigInterval  time.Duration
	publishLogsInterval    time.Duration

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

func (h *HostSimulation) Enroll() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	nodeKey, invalid, err := h.state.serviceClient.RequestEnrollment(context.Background(), h.enrollSecret, h.uuid)
	if err != nil {
		return errors.Wrap(err, "transport error in enrollment")
	}
	if invalid {
		return errors.New("enrollment invalid")
	}

	h.state.nodeKey = nodeKey
	return nil
}

func (h *HostSimulation) RequestConfig() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	config, invalid, err := h.state.serviceClient.RequestConfig(context.Background(), h.state.nodeKey)
	if err != nil {
		return errors.Wrap(err, "transport error retrieving config")
	}
	if invalid {
		return errors.New("enrollment invalid in request config")
	}

	// Ignore actual config. Someday we may do something with the config.
	_ = config
	return nil
}

func (h *HostSimulation) PublishLogs() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	logs := []string{"foo", "bar", "baz"}
	_, _, invalid, err := h.state.serviceClient.PublishLogs(context.Background(), h.state.nodeKey, logger.LogTypeStatus, logs)
	if err != nil {
		return errors.Wrap(err, "transport error publishing logs")
	}
	if invalid {
		return errors.New("enrollment invalid in publish logs")
	}
	return nil
}

func (h *HostSimulation) RequestQueries() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	queries, invalid, err := h.state.serviceClient.RequestQueries(context.Background(), h.state.nodeKey)
	if err != nil {
		return errors.Wrap(err, "transport error requesting queries")
	}
	if invalid {
		return errors.New("enrollment invalid in request queries")
	}

	if len(queries.Queries) == 0 {
		// No queries to run
		return nil
	}

	results := []distributed.Result{}
	for name, sql := range queries.Queries {
		rows, err := h.host.RunQuery(sql)
		if err != nil {
			rows = []map[string]string{}
		}
		results = append(results,
			distributed.Result{QueryName: name, Status: 0, Rows: rows},
		)
	}

	_, _, invalid, err = h.state.serviceClient.PublishResults(context.Background(), h.state.nodeKey, results)
	if err != nil {
		return errors.Wrap(err, "transport error publishing results")
	}
	if invalid {
		return errors.New("enrollment invalid in publish results")
	}

	return nil
}

// hostSimulationState is a light container around simulation state management
type hostSimulationState struct {
	lock          sync.RWMutex
	serviceClient service.KolideService
	nodeKey       string
	failed        bool
	started       bool
}

// SimulationOption is a functional option pattern for defining how a host
// simulation instance should be configured. For more information on this
// patter, see the following blog post:
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type SimulationOption func(*HostSimulation)

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

// WithPublishLogsInterval is a functional option which allows the user to
// specify how often the simulation host should log status and results logs
func WithPublishLogsInterval(interval time.Duration) SimulationOption {
	return func(i *HostSimulation) {
		i.publishLogsInterval = interval
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

// WithUUID is a functional option that sets the UUID the fake host provides to
// the server.
func WithUUID() SimulationOption {
	return func(i *HostSimulation) {
		i.insecureGrpc = true
	}
}

// createSimulationRuntime is an internal helper which creates an instance of
// *HostSimulation given a set of supplied functional options
func createSimulationRuntime(host QueryRunner, uuid, enrollSecret string, opts ...SimulationOption) *HostSimulation {
	h := &HostSimulation{
		host:                   host,
		uuid:                   uuid,
		enrollSecret:           enrollSecret,
		requestQueriesInterval: 2 * time.Second,
		requestConfigInterval:  5 * time.Second,
		publishLogsInterval:    10 * time.Second,
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
func LaunchSimulation(host QueryRunner, grpcURL, uuid, enrollSecret string, opts ...SimulationOption) *HostSimulation {
	h := createSimulationRuntime(host, uuid, enrollSecret, opts...)
	fmt.Println(h)
	go func() {
		h.state.lock.Lock()
		h.state.started = true

		fmt.Println("in goroutine")
		grpcOpts := []grpc.DialOption{
			grpc.WithTimeout(time.Second),
		}
		if h.insecureGrpc {
			grpcOpts = append(grpcOpts, grpc.WithInsecure())
		} else {
			host, _, err := net.SplitHostPort(grpcURL)
			if err != nil {
				err = errors.Wrapf(err, "split grpc server host and port: %s", grpcURL)
				h.state.failed = true
				h.state.lock.Unlock()
				return
			}
			creds := credentials.NewTLS(&tls.Config{
				ServerName:         host,
				InsecureSkipVerify: h.insecure,
			})
			grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))
		}
		conn, err := grpc.Dial(grpcURL, grpcOpts...)
		if err != nil {
			fmt.Println(err)
			h.state.failed = true
			h.state.lock.Unlock()
			return
		}
		defer conn.Close()
		fmt.Println("created conn")

		h.state.serviceClient = service.New(conn, log.NewNopLogger())

		h.state.lock.Unlock()

		err = h.Enroll()
		fmt.Println("enroll", err)
		if err != nil {
			h.state.lock.Lock()
			h.state.failed = true
			h.state.lock.Unlock()
			return
		}

		requestQueriesTicker := time.NewTicker(h.requestQueriesInterval)
		requestConfigTicker := time.NewTicker(h.requestConfigInterval)
		publishLogsTicker := time.NewTicker(h.publishLogsInterval)

		for {
			var err error
			select {
			case <-requestQueriesTicker.C:
				err = h.RequestQueries()
			case <-requestConfigTicker.C:
				err = h.RequestConfig()
			case <-publishLogsTicker.C:
				err = h.PublishLogs()
			case <-h.shutdown:
				h.done <- true
				return
			}
			if err != nil {
				h.state.lock.Lock()
				h.state.failed = true
				h.state.lock.Unlock()
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

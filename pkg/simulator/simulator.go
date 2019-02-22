package simulator

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/service"
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
	logger                 log.Logger

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

	shutdown chan chan struct{}
}

// Enroll is the implementation of the host simulation's enrollment functionality
func (h *HostSimulation) Enroll() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	enrollmentAttempts := 5

	var err error
	for currentAttempt := 1; currentAttempt <= enrollmentAttempts; currentAttempt++ {
		if currentAttempt != 1 {
			level.Debug(h.logger).Log(
				"msg", "first enrollment attempt failed, re-trying enrollment",
				"attempt", currentAttempt,
			)
			time.Sleep(time.Duration(math.Pow(2, float64(currentAttempt))) * time.Second)
		}
		nodeKey, invalid, err := h.state.serviceClient.RequestEnrollment(context.Background(), h.enrollSecret, h.uuid, service.EnrollmentDetails{})
		if err != nil {
			level.Debug(h.logger).Log(
				"msg", "transport error in enrollment",
				"err", err.Error(),
				"uuid", h.uuid,
			)
			err = errors.Wrap(err, "transport error in enrollment")
			continue
		}
		if invalid {
			level.Debug(h.logger).Log(
				"msg", "server responded that enrollment was invalid",
				"uuid", h.uuid,
			)
			err = fmt.Errorf("enrollment invalid for host with uuid: %s", h.uuid)
			continue
		}
		h.state.nodeKey = nodeKey
		return nil
	}

	level.Debug(h.logger).Log(
		"msg", "successfully enrolled host",
		"uuid", h.uuid,
	)

	return err
}

// RequestConfig is the implementation of the host simulation's config retrieval
// functionality
func (h *HostSimulation) RequestConfig() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	// TODO: When we request the config, we are throwing away what the server
	// returns. An enhancement to this simulator should take the packs in the
	// config and execute the queries on a realistic schedule.
	// Further, the config may contain options as well which could influence the
	// desired rate at which a host should be checking in to the server which has
	// an obvious effect on the integrity of a load test.
	config, invalid, err := h.state.serviceClient.RequestConfig(context.Background(), h.state.nodeKey)
	if err != nil {
		level.Debug(h.logger).Log(
			"msg", "transport error requesting config",
			"err", err.Error(),
			"uuid", h.uuid,
		)
		return errors.Wrap(err, "transport error retrieving config")
	}
	if invalid {
		level.Debug(h.logger).Log(
			"msg", "server responded that config request was invalid",
			"uuid", h.uuid,
		)
		return errors.New("enrollment invalid in request config")
	}

	level.Debug(h.logger).Log(
		"msg", "successfully requested config",
		"config", config,
		"uuid", h.uuid,
	)

	return nil
}

// PublishLogs is the implementation of the host simulation's log publishing
// functionality
func (h *HostSimulation) PublishLogs() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	logs := []string{"foo", "bar", "baz"}
	_, _, invalid, err := h.state.serviceClient.PublishLogs(context.Background(), h.state.nodeKey, logger.LogTypeStatus, logs)
	if err != nil {
		level.Debug(h.logger).Log(
			"msg", "transport error publishing logs",
			"err", err.Error(),
			"uuid", h.uuid,
		)
		return errors.Wrap(err, "transport error publishing logs")
	}
	if invalid {
		level.Debug(h.logger).Log(
			"msg", "server responded that log publish was invalid",
			"uuid", h.uuid,
		)
		return errors.New("enrollment invalid in publish logs")
	}

	level.Debug(h.logger).Log(
		"msg", "successfully published logs",
		"uuid", h.uuid,
	)

	return nil
}

// RequestQueries is the implementation of the host simulation's request new
// queries functionality
func (h *HostSimulation) RequestQueries() error {
	h.state.lock.Lock()
	defer h.state.lock.Unlock()

	queries, invalid, err := h.state.serviceClient.RequestQueries(context.Background(), h.state.nodeKey)
	if err != nil {
		level.Debug(h.logger).Log(
			"msg", "transport error requesting queries",
			"err", err.Error(),
			"uuid", h.uuid,
		)
		return errors.Wrap(err, "transport error requesting queries")
	}
	if invalid {
		level.Debug(h.logger).Log(
			"msg", "server responded that requesting queries was invalid",
			"uuid", h.uuid,
		)
		return errors.New("enrollment invalid in request queries")
	}

	if len(queries.Queries) == 0 {
		// No queries to run
		return nil
	}

	results := []distributed.Result{}
	for name, sql := range queries.Queries {
		level.Debug(h.logger).Log(
			"msg", "running live query",
			"name", name,
			"sql", sql,
			"uuid", h.uuid,
		)
		rows, err := h.host.RunQuery(sql)
		if err != nil {
			level.Debug(h.logger).Log(
				"msg", "error running query",
				"err", err.Error(),
				"uuid", h.uuid,
			)
			rows = []map[string]string{}
		}
		results = append(results,
			distributed.Result{QueryName: name, Status: 0, Rows: rows},
		)
	}

	level.Debug(h.logger).Log(
		"msg", "publishing results for live queries",
		"uuid", h.uuid,
	)

	_, _, invalid, err = h.state.serviceClient.PublishResults(context.Background(), h.state.nodeKey, results)
	if err != nil {
		level.Debug(h.logger).Log(
			"msg", "transport error publishing distributed query results",
			"err", err.Error(),
			"uuid", h.uuid,
		)
		return errors.Wrap(err, "transport error publishing results")
	}
	if invalid {
		level.Debug(h.logger).Log(
			"msg", "server responded that publishing distributed query results was invalid",
			"uuid", h.uuid,
		)
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

// WithInsecure is a functional option which allows the user to declare that
// when connecting to the remote API, the client should skip TLS certificate
// verification but still use TLS
func WithInsecure() SimulationOption {
	return func(i *HostSimulation) {
		i.insecure = true
	}
}

// WithInsecureGrpc is a functional option which allows the user to declare that
// WithInsecure is a functional option which allows the user to declare that
// when connecting to the remote API, the client should turn off TLS entirely
func WithInsecureGrpc() SimulationOption {
	return func(i *HostSimulation) {
		i.insecureGrpc = true
	}
}

// createSimulationRuntime is an internal helper which creates an instance of
// *HostSimulation given a set of supplied functional options
func createSimulationRuntime(logger log.Logger, host QueryRunner, uuid, enrollSecret string, opts ...SimulationOption) *HostSimulation {
	h := &HostSimulation{
		logger:                 logger,
		host:                   host,
		uuid:                   uuid,
		enrollSecret:           enrollSecret,
		requestQueriesInterval: 2 * time.Second,
		requestConfigInterval:  5 * time.Second,
		publishLogsInterval:    10 * time.Second,
		shutdown:               make(chan chan struct{}),
		state:                  &hostSimulationState{},
	}
	for _, opt := range opts {
		opt(h)
	}

	return h
}

// LaunchSimulation is a utility which allows the user to configure and run an
// asynchronous instance of a host simulation given a set of functional options
func LaunchSimulation(logger log.Logger, host QueryRunner, grpcURL, uuid, enrollSecret string, opts ...SimulationOption) *HostSimulation {
	h := createSimulationRuntime(logger, host, uuid, enrollSecret, opts...)
	go func() {
		h.state.lock.Lock()
		h.state.started = true

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
			h.state.failed = true
			h.state.lock.Unlock()
			return
		}
		defer conn.Close()

		h.state.serviceClient = service.New(conn, log.NewNopLogger())

		h.state.lock.Unlock()

		err = h.Enroll()
		if err != nil {
			h.state.lock.Lock()
			level.Debug(h.logger).Log(
				"msg", "enrollment failed",
				"uuid", h.uuid,
			)
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
			case done := <-h.shutdown:
				close(done)
				return
			}
			if err != nil {
				level.Debug(h.logger).Log(
					"msg", "an error occurred in simulation",
					"err", err.Error(),
					"uuid", h.uuid,
				)
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

// Shutdown will attempt to gracefully shutdown the simulation
func (h *HostSimulation) Shutdown() error {
	done := make(chan struct{})
	h.shutdown <- done

	timer := time.NewTimer(time.Second)
	select {
	case <-done:
		return nil
	case <-timer.C:
	}

	return errors.New("simulation did not shut down in time")
}

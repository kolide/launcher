package enroller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/traces"
)

type enroller struct {
	slogger *slog.Logger
	k       types.Knapsack
	client  enrollmentClient

	enrollLock *sync.Mutex

	interrupt   chan struct{}
	interrupted bool
}

// enrollmentClient interface is a subset of the service.KolideService interface, allowing us to swap
// to enrolling via e.g. control server in the future instead
type enrollmentClient interface {
	// RequestEnrollment requests a node key for the host, authenticating
	// with the given enroll secret.
	RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error)
}

const (
	queryDetailsTimeout       = 30 * time.Second
	queryDetailsRetryInterval = 5 * time.Second
	enrollmentRetryInterval   = queryDetailsTimeout + 1*time.Minute

	configStoreKeyForNodeKey = "nodeKey"
)

func New(k types.Knapsack, client enrollmentClient) *enroller {
	return &enroller{
		slogger:    k.Slogger().With("component", "enroller"),
		k:          k,
		client:     client,
		enrollLock: &sync.Mutex{},
		interrupt:  make(chan struct{}, 1),
	}
}

func (e *enroller) Run() error {
	retryTicker := time.NewTicker(enrollmentRetryInterval)
	defer retryTicker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), enrollmentRetryInterval)
		err := e.enrollIfNotAlreadyEnrolled(ctx)
		if err == nil {
			e.slogger.Log(ctx, slog.LevelInfo,
				"already enrolled, or enrollment successful",
			)
			cancel()
			break
		}

		e.slogger.Log(ctx, slog.LevelError,
			"enrollment attempt unsuccessful",
			"err", err,
		)
		cancel()

		select {
		case <-retryTicker.C:
			continue
		case <-e.interrupt:
			e.slogger.Log(context.TODO(), slog.LevelDebug,
				"received external interrupt before enrollment has completed -- stopping",
			)
			return nil
		}
	}

	// Enrollment has completed. Wait until rungroup exits.
	<-e.interrupt
	return nil
}

func (e *enroller) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if e.interrupted {
		return
	}
	e.interrupted = true

	e.interrupt <- struct{}{}
}

func (e *enroller) enrollIfNotAlreadyEnrolled(ctx context.Context) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	alreadyEnrolled, err := e.enrolled(ctx)
	if err != nil {
		return fmt.Errorf("could not determine if already enrolled: %w", err)
	}
	if alreadyEnrolled {
		e.slogger.Log(ctx, slog.LevelInfo,
			"key found, no need to enroll",
		)
		return nil
	}

	e.slogger.Log(ctx, slog.LevelInfo,
		"key not found, proceeding with enrollment",
	)

	return e.enroll(ctx)
}

func (e *enroller) enrolled(ctx context.Context) (bool, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	key, err := e.k.ConfigStore().Get([]byte(configStoreKeyForNodeKey))
	if err != nil {
		return false, fmt.Errorf("getting node key: %w", err)
	}
	return key != nil, nil
}

func (e *enroller) enroll(ctx context.Context) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	e.enrollLock.Lock()
	defer e.enrollLock.Unlock()

	enrollSecret, err := e.readEnrollSecret(ctx)
	if err != nil {
		return fmt.Errorf("could not read enroll secret: %w", err)
	}

	hostIdentifier, err := osquery.IdentifierFromDB(e.k.ConfigStore())
	if err != nil {
		traces.SetError(span, fmt.Errorf("error getting host identifier: %w", err))
		return fmt.Errorf("could not get host identifier: %w", err)
	}

	enrollmentDetails := e.queryEnrollmentDetails(ctx)

	nodeKey, nodeInvalid, err := e.client.RequestEnrollment(ctx, enrollSecret, hostIdentifier, enrollmentDetails)
	if err != nil {
		return fmt.Errorf("error requesting enrollment with node invalid %v: %w", nodeInvalid, err)
	}
	if nodeInvalid {
		return errors.New("received invalid node response when requesting enrollment")
	}

	err = e.k.ConfigStore().Set([]byte(configStoreKeyForNodeKey), []byte(nodeKey))
	if err != nil {
		return fmt.Errorf("could not save node key after enrollment: %w", err)
	}

	// TODO notify via knapsack

	return nil
}

func (e *enroller) readEnrollSecret(ctx context.Context) (string, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	if e.k.EnrollSecret() != "" {
		return e.k.EnrollSecret(), nil
	}

	if e.k.EnrollSecretPath() != "" {
		content, err := os.ReadFile(e.k.EnrollSecretPath())
		if err != nil {
			return "", fmt.Errorf("could not read enroll secret path %s: %w", e.k.EnrollSecretPath(), err)
		}
		return string(bytes.TrimSpace(content)), nil
	}

	return "", errors.New("enroll secret not set")
}

func (e *enroller) queryEnrollmentDetails(ctx context.Context) service.EnrollmentDetails {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	var enrollDetails service.EnrollmentDetails

	osqPath := e.k.LatestOsquerydPath(ctx)
	if osqPath == "" {
		e.slogger.Log(ctx, slog.LevelInfo,
			"no osquery path found, skipping enrollment details query",
		)
		return enrollDetails
	}

	var err error
	if err := backoff.WaitFor(func() error {
		enrollDetails, err = osquery.GetEnrollDetails(ctx, osqPath)
		return err
	}, queryDetailsTimeout, queryDetailsRetryInterval); err != nil {
		e.slogger.Log(ctx, slog.LevelError,
			"failed to get enrollment details with retries, proceeding without them",
			"err", err,
		)
	}

	return enrollDetails
}

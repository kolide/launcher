package osquery

import (
	"context"

	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

// Extension is the implementation of the osquery extension methods. It handles
// both the communication with the osquery daemon and the Kolide server.
type Extension struct {
	NodeKey       string
	serviceClient service.KolideService
}

// NewExtension creates a new Extension from the provided service.KolideService
// implementation.
func NewExtension(client service.KolideService) (*Extension, error) {
	return &Extension{
		serviceClient: client,
	}, nil
}

// TODO this should come from something sensible
const version = "foobar"

// Enroll will attempt to enroll the host using the provided enroll secret for
// identification. In the future it should look for existing enrollment
// configuration locally.
func (e *Extension) Enroll(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
	key, invalid, err := e.serviceClient.RequestEnrollment(context.Background(), enrollSecret, hostIdentifier)
	if err != nil {
		return "", true, errors.Wrap(err, "transport error in enrollment")
	}
	if invalid {
		return "", true, errors.New("enrollment invalid")
	}

	e.NodeKey = key
	return key, invalid, err
}

// GenerateConfigs will request the osquery configuration from the server. In
// the future it should look for existing configuration locally.
func (e *Extension) GenerateConfigs(ctx context.Context) (map[string]string, error) {
	// TODO get version
	config, invalid, err := e.serviceClient.RequestConfig(ctx, e.NodeKey, version)
	if err != nil {
		return nil, errors.Wrap(err, "transport error retrieving config")
	}

	if invalid {
		return nil, errors.New("enrollment invalid")
	}

	return map[string]string{"config": config}, nil
}

// LogString will publish a status/result log from osquery to the server. In
// the future it should buffer logs and send them at intervals.
func (e *Extension) LogString(ctx context.Context, typ logger.LogType, logText string) error {
	// TODO get version
	_, _, invalid, err := e.serviceClient.PublishLogs(ctx, e.NodeKey, version, typ, []string{logText})
	if err != nil {
		return errors.Wrap(err, "transport error sending logs")
	}

	if invalid {
		return errors.New("enrollment invalid")
	}

	return nil
}

// GetQueries will request the distributed queries to execute from the server.
func (e *Extension) GetQueries(ctx context.Context) (*distributed.GetQueriesResult, error) {
	queries, invalid, err := e.serviceClient.RequestQueries(ctx, e.NodeKey, version)
	if err != nil {
		return nil, errors.Wrap(err, "transport error getting queries")
	}

	if invalid {
		return nil, errors.New("enrollment invalid")
	}

	return queries, nil
}

// WriteResults will publish results of the executed distributed queries back
// to the server.
func (e *Extension) WriteResults(ctx context.Context, results []distributed.Result) error {
	_, _, invalid, err := e.serviceClient.PublishResults(ctx, e.NodeKey, results)
	if err != nil {
		return errors.Wrap(err, "transport error writing results")
	}

	if invalid {
		return errors.New("enrollment invalid")
	}

	return nil
}

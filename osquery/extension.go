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

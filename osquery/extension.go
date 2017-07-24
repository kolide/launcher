package osquery

import (
	"context"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

// Extension is the implementation of the osquery extension methods. It handles
// both the communication with the osquery daemon and the Kolide server.
type Extension struct {
	NodeKey       string
	EnrollSecret  string
	db            *bolt.DB
	serviceClient service.KolideService
	enrollMutex   sync.Mutex
}

// bucketNames is the name of buckets that should be created when the extension
// opens the DB. It should be treated as a constant.
var bucketNames = []string{configBucket}

// ExtensionOpts is options to be passed in NewExtension
type ExtensionOpts struct {
	// EnrollSecret is the (mandatory) enroll secret used for
	// enrolling with the server.
	EnrollSecret string
}

// NewExtension creates a new Extension from the provided service.KolideService
// implementation.
func NewExtension(client service.KolideService, db *bolt.DB, opts ExtensionOpts) (*Extension, error) {
	if opts.EnrollSecret == "" {
		return nil, errors.New("empty enroll secret")
	}

	// Create Bolt buckets as necessary
	err := db.Update(func(tx *bolt.Tx) error {
		for _, name := range bucketNames {
			_, err := tx.CreateBucketIfNotExists([]byte(name))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating DB buckets")
	}

	return &Extension{
		serviceClient: client,
		db:            db,
		EnrollSecret:  opts.EnrollSecret,
	}, nil
}

// Bucket name to use for launcher configuration.
const configBucket = "config"

// DB key for node key
const nodeKeyKey = "nodeKey"

// Enroll will attempt to enroll the host using the provided enroll secret for
// identification. If the host is already enrolled, the existing node key will
// be returned. To force re-enrollment, use RequireReenroll.
func (e *Extension) Enroll(ctx context.Context, hostIdentifier string) (string, bool, error) {
	// Only one thread should ever be allowed to attempt enrollment at the
	// same time.
	e.enrollMutex.Lock()
	defer e.enrollMutex.Unlock()

	// If we already have a successful enrollment (perhaps from another
	// thread), no need to do anything else.
	if e.NodeKey != "" {
		return e.NodeKey, false, nil
	}

	// Look up a node key cached in the local store
	var key []byte
	e.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(configBucket))
		key = b.Get([]byte(nodeKeyKey))
		return nil

	})
	if key != nil {
		e.NodeKey = string(key)
		return e.NodeKey, false, nil
	}

	// If no cached node key, enroll for new node key
	keyString, invalid, err := e.serviceClient.RequestEnrollment(context.Background(), e.EnrollSecret, hostIdentifier)
	if err != nil {
		return "", true, errors.Wrap(err, "transport error in enrollment")
	}
	if invalid {
		return "", true, errors.New("enrollment invalid")
	}

	// Save newly acquired node key if successful
	err = e.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(configBucket))
		return b.Put([]byte(nodeKeyKey), []byte(keyString))
	})
	if err != nil {
		return "", true, errors.Wrap(err, "saving node key")
	}

	e.NodeKey = keyString
	return e.NodeKey, false, nil
}

// RequireReenroll clears the existing node key information, ensuring that the
// next call to Enroll will cause the enrollment process to take place.
func (e *Extension) RequireReenroll(ctx context.Context) {
	e.enrollMutex.Lock()
	defer e.enrollMutex.Unlock()
	// Clear the node key such that reenrollment is required.
	e.NodeKey = ""
	e.db.Update(func(tx *bolt.Tx) error {
		tx.Bucket([]byte(configBucket)).Delete([]byte(nodeKeyKey))
		return nil
	})
}

// GenerateConfigs will request the osquery configuration from the server. In
// the future it should look for existing configuration locally.
func (e *Extension) GenerateConfigs(ctx context.Context) (map[string]string, error) {
	return e.generateConfigsWithReenroll(ctx, true)
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) generateConfigsWithReenroll(ctx context.Context, reenroll bool) (map[string]string, error) {
	config, invalid, err := e.serviceClient.RequestConfig(ctx, e.NodeKey)
	if err != nil {
		return nil, errors.Wrap(err, "transport error retrieving config")
	}

	if invalid {
		if !reenroll {
			return nil, errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx, "foobar")
		if err != nil {
			return nil, errors.Wrap(err, "enrollment invalid, reenrollment errored")
		}
		if invalid {
			return nil, errors.New("enrollment invalid, reenrollment invalid")
		}

		// Don't attempt reenroll after first attempt
		return e.generateConfigsWithReenroll(ctx, false)
	}

	return map[string]string{"config": config}, nil

}

// LogString will publish a status/result log from osquery to the server. In
// the future it should buffer logs and send them at intervals.
func (e *Extension) LogString(ctx context.Context, typ logger.LogType, logText string) error {
	return e.logStringWithReenroll(ctx, typ, logText, true)
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) logStringWithReenroll(ctx context.Context, typ logger.LogType, logText string, reenroll bool) error {
	_, _, invalid, err := e.serviceClient.PublishLogs(ctx, e.NodeKey, typ, []string{logText})
	if err != nil {
		return errors.Wrap(err, "transport error sending logs")
	}

	if invalid {
		if !reenroll {
			return errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx, "foobar")
		if err != nil {
			return errors.Wrap(err, "enrollment invalid, reenrollment errored")
		}
		if invalid {
			return errors.New("enrollment invalid, reenrollment invalid")
		}

		// Don't attempt reenroll after first attempt
		return e.logStringWithReenroll(ctx, typ, logText, false)
	}

	return nil
}

// GetQueries will request the distributed queries to execute from the server.
func (e *Extension) GetQueries(ctx context.Context) (*distributed.GetQueriesResult, error) {
	return e.getQueriesWithReenroll(ctx, true)
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) getQueriesWithReenroll(ctx context.Context, reenroll bool) (*distributed.GetQueriesResult, error) {
	queries, invalid, err := e.serviceClient.RequestQueries(ctx, e.NodeKey)
	if err != nil {
		return nil, errors.Wrap(err, "transport error getting queries")
	}

	if invalid {
		if !reenroll {
			return nil, errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx, "foobar")
		if err != nil {
			return nil, errors.Wrap(err, "enrollment invalid, reenrollment errored")
		}
		if invalid {
			return nil, errors.New("enrollment invalid, reenrollment invalid")
		}

		// Don't attempt reenroll after first attempt
		return e.getQueriesWithReenroll(ctx, false)
	}

	return queries, nil
}

// WriteResults will publish results of the executed distributed queries back
// to the server.
func (e *Extension) WriteResults(ctx context.Context, results []distributed.Result) error {
	return e.writeResultsWithReenroll(ctx, results, true)
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) writeResultsWithReenroll(ctx context.Context, results []distributed.Result, reenroll bool) error {
	_, _, invalid, err := e.serviceClient.PublishResults(ctx, e.NodeKey, results)
	if err != nil {
		return errors.Wrap(err, "transport error writing results")
	}

	if invalid {
		if !reenroll {
			return errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx, "foobar")
		if err != nil {
			return errors.Wrap(err, "enrollment invalid, reenrollment errored")
		}
		if invalid {
			return errors.New("enrollment invalid, reenrollment invalid")
		}

		// Don't attempt reenroll after first attempt
		return e.writeResultsWithReenroll(ctx, results, false)
	}

	return nil
}

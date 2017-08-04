package osquery

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/mixer/clock"
	"github.com/pkg/errors"
)

// Extension is the implementation of the osquery extension methods. It handles
// both the communication with the osquery daemon and the Kolide server.
type Extension struct {
	NodeKey       string
	Opts          ExtensionOpts
	db            *bolt.DB
	serviceClient service.KolideService
	enrollMutex   sync.Mutex
	done          chan struct{}
	wg            sync.WaitGroup
	logger        log.Logger
}

const (
	// Bucket name to use for launcher configuration.
	configBucket = "config"
	// Bucket name to use for buffered status logs.
	statusLogsBucket = "status_logs"
	// Bucket name to use for buffered result logs.
	resultLogsBucket = "result_logs"

	// DB key for UUID
	uuidKey = "uuid"
	// DB key for node key
	nodeKeyKey = "nodeKey"
	// DB key for last retrieved config
	configKey = "config"

	// Default maximum number of logs per batch (used if not specified in
	// options)
	defaultMaxLogsPerBatch = 500
	// Default logging interval (used if not specified in
	// options)
	defaultLoggingInterval = 1 * time.Minute
	// Default maximum number of logs to buffer before purging oldest logs
	// (applies per log type).
	defaultMaxBufferedLogs = 500000
)

// ExtensionOpts is options to be passed in NewExtension
type ExtensionOpts struct {
	// EnrollSecret is the (mandatory) enroll secret used for
	// enrolling with the server.
	EnrollSecret string
	// MaxLogsPerBatch is the maximum number of logs that should be sent in
	// one batch logging request.
	MaxLogsPerBatch int
	// LoggingInterval is the interval at which logs should be flushed to
	// the server.
	LoggingInterval time.Duration
	// Clock is the clock that should be used for time based operations. By
	// default it will be a normal realtime clock, but a mock clock can be
	// passed with clock.NewMockClock() for testing purposes.
	Clock clock.Clock
	// Logger is the logger that the extension should use. This is for
	// logging about the launcher, and not for logging osquery results.
	Logger log.Logger
	// MaxBufferedLogs is the maximum number of logs to buffer before
	// purging oldest logs (applies per log type).
	MaxBufferedLogs int
}

// NewExtension creates a new Extension from the provided service.KolideService
// implementation. The background routines should be started by calling
// Start().
func NewExtension(client service.KolideService, db *bolt.DB, opts ExtensionOpts) (*Extension, error) {
	// bucketNames contains the names of buckets that should be created when the
	// extension opens the DB. It should be treated as a constant.
	var bucketNames = []string{configBucket, statusLogsBucket, resultLogsBucket}

	if opts.EnrollSecret == "" {
		return nil, errors.New("empty enroll secret")
	}

	if opts.MaxLogsPerBatch == 0 {
		opts.MaxLogsPerBatch = defaultMaxLogsPerBatch
	}

	if opts.LoggingInterval == 0 {
		opts.LoggingInterval = defaultLoggingInterval
	}

	if opts.Clock == nil {
		opts.Clock = clock.DefaultClock{}
	}

	if opts.Logger == nil {
		// Nop logger
		opts.Logger = log.LoggerFunc(func(...interface{}) error {
			return nil
		})
	}

	if opts.MaxBufferedLogs == 0 {
		opts.MaxBufferedLogs = defaultMaxBufferedLogs
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
		Opts:          opts,
		done:          make(chan struct{}),
	}, nil
}

// Start begins the goroutines responsible for background processing (currently
// just the log buffer flushing routine). It should be shut down by calling the
// Shutdown() method.
func (e *Extension) Start() {
	e.wg.Add(1)
	go e.writeLogsLoopRunner()
}

// Shutdown should be called to cleanup the resources and goroutines associated
// with this extension.
func (e *Extension) Shutdown() {
	close(e.done)
	e.wg.Wait()
}

// getHostIdentifier returns the UUID identifier associated with this host. If
// there is an existing identifier, that should be returned. If not, the
// identifier should be randomly generated and persisted.
func (e *Extension) getHostIdentifier() (string, error) {
	var identifier string
	err := e.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(configBucket))
		uuidBytes := b.Get([]byte(uuidKey))
		gotID, err := uuid.ParseBytes(uuidBytes)

		// Use existing UUID
		if err == nil {
			identifier = gotID.String()
			return nil
		}

		// Generate new (random) UUID
		gotID, err = uuid.NewRandom()
		if err != nil {
			return errors.Wrap(err, "generating new UUID")
		}
		identifier = gotID.String()

		// Save new UUID
		err = b.Put([]byte(uuidKey), []byte(identifier))
		return errors.Wrap(err, "saving new UUID")
	})

	if err != nil {
		return "", err
	}

	return identifier, nil
}

// Enroll will attempt to enroll the host using the provided enroll secret for
// identification. If the host is already enrolled, the existing node key will
// be returned. To force re-enrollment, use RequireReenroll.
func (e *Extension) Enroll(ctx context.Context) (string, bool, error) {
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

	identifier, err := e.getHostIdentifier()
	if err != nil {
		return "", true, errors.Wrap(err, "generating UUID")
	}

	// If no cached node key, enroll for new node key
	keyString, invalid, err := e.serviceClient.RequestEnrollment(context.Background(), e.Opts.EnrollSecret, identifier)
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

// GenerateConfigs will request the osquery configuration from the server. If
// retrieving the configuration from the server fails, the locally stored
// configuration will be returned. If that fails, this method will return an
// error.
func (e *Extension) GenerateConfigs(ctx context.Context) (map[string]string, error) {
	config, err := e.generateConfigsWithReenroll(ctx, true)
	if err != nil {
		// Try to use cached config
		var confBytes []byte
		e.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(configBucket))
			confBytes = b.Get([]byte(configKey))
			return nil
		})

		if len(confBytes) == 0 {
			return nil, errors.Wrap(err, "loading config failed, no cached config")
		}
		config = string(confBytes)
	} else {
		// Store good config
		e.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(configBucket))
			return b.Put([]byte(configKey), []byte(config))
		})
		// TODO log or record metrics when caching config fails? We
		// would probably like to return the config and not an error in
		// this case.
	}

	return map[string]string{"config": config}, nil
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) generateConfigsWithReenroll(ctx context.Context, reenroll bool) (string, error) {
	config, invalid, err := e.serviceClient.RequestConfig(ctx, e.NodeKey)
	if err != nil {
		return "", errors.Wrap(err, "transport error retrieving config")
	}

	if invalid {
		if !reenroll {
			return "", errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx)
		if err != nil {
			return "", errors.Wrap(err, "enrollment invalid, reenrollment errored")
		}
		if invalid {
			return "", errors.New("enrollment invalid, reenrollment invalid")
		}

		// Don't attempt reenroll after first attempt
		return e.generateConfigsWithReenroll(ctx, false)
	}

	return config, nil

}

// byteKeyFromUint64 turns a uint64 (generated by Bolt's NextSequence) into a
// sortable byte slice to use as a key.
func byteKeyFromUint64(k uint64) []byte {
	// Adapted from Bolt docs
	// 8 bytes in a uint64
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, k)
	return b
}

// uint64FromByteKey turns a byte slice (retrieved as the key from Bolt) into a
// uint64
func uint64FromByteKey(k []byte) uint64 {
	return binary.BigEndian.Uint64(k)
}

// bucketNameFromLogType returns the Bolt bucket name that stores logs of the
// provided type.
func bucketNameFromLogType(typ logger.LogType) (string, error) {
	switch typ {
	case logger.LogTypeString, logger.LogTypeSnapshot:
		return resultLogsBucket, nil
	case logger.LogTypeStatus:
		return statusLogsBucket, nil
	default:
		return "", errors.Errorf("unknown log type: %v", typ)

	}
}

// writeAndPurgeLogs flushes the log buffers, writing up to
// Opts.MaxLogsPerBatch in one run. If the logs write successfully, they will
// be deleted from the buffer. After writing (whether success or failure), logs
// over the maximum count will be purged to avoid unbounded growth of the
// buffers.
func (e *Extension) writeAndPurgeLogs() {
	for _, typ := range []logger.LogType{logger.LogTypeStatus, logger.LogTypeString} {
		// Write logs
		err := e.writeBufferedLogsForType(typ)
		if err != nil {
			level.Info(e.Opts.Logger).Log(
				"err",
				errors.Wrapf(err, "sending %v logs", typ),
			)
		}

		// Purge overflow
		err = e.purgeBufferedLogsForType(typ)
		if err != nil {
			level.Info(e.Opts.Logger).Log(
				"err",
				errors.Wrapf(err, "purging %v logs", typ),
			)
		}
	}
}

func (e *Extension) writeLogsLoopRunner() {
	defer e.wg.Done()
	ticker := e.Opts.Clock.NewTicker(e.Opts.LoggingInterval)
	defer ticker.Stop()
	for {
		e.writeAndPurgeLogs()

		// select to either exit or write another batch of logs
		select {
		case <-e.done:
			return
		case <-ticker.Chan():
			// Resume loop
		}
	}
}

// writeBufferedLogs flushes the log buffers, writing up to
// Opts.MaxLogsPerBatch in one run. If the logs write successfully, they will
// be deleted from the buffer.
func (e *Extension) writeBufferedLogsForType(typ logger.LogType) error {
	bucketName, err := bucketNameFromLogType(typ)
	if err != nil {
		return err
	}
	err = e.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))

		logs := []string{}
		c := b.Cursor()
		k, v := c.First()
		for total := 0; k != nil && total < e.Opts.MaxLogsPerBatch; total++ {
			logs = append(logs, string(v))
			c.Delete() // Note: This advances the cursor
			k, v = c.First()
		}

		if len(logs) == 0 {
			// Nothing to send
			return nil
		}

		err := e.writeLogsWithReenroll(context.Background(), typ, logs, true)
		if err != nil {
			// Returning an error will cancel the
			// transaction and the logs will not be
			// deleted.
			return errors.Wrap(err, "writing logs")
		}

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "writing buffered logs")
	}
	return nil
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) writeLogsWithReenroll(ctx context.Context, typ logger.LogType, logs []string, reenroll bool) error {
	_, _, invalid, err := e.serviceClient.PublishLogs(ctx, e.NodeKey, typ, logs)
	if err != nil {
		return errors.Wrap(err, "transport error sending logs")
	}

	if invalid {
		if !reenroll {
			return errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx)
		if err != nil {
			return errors.Wrap(err, "enrollment invalid, reenrollment errored")
		}
		if invalid {
			return errors.New("enrollment invalid, reenrollment invalid")
		}

		// Don't attempt reenroll after first attempt
		return e.writeLogsWithReenroll(ctx, typ, logs, false)
	}

	return nil
}

// purgeBufferedLogs flushes the log buffers, ensuring that at most
// Opts.MaxBufferedLogs logs remain for each log type.
func (e *Extension) purgeBufferedLogsForType(typ logger.LogType) error {
	bucketName, err := bucketNameFromLogType(typ)
	if err != nil {
		return err
	}
	err = e.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))

		logCount := b.Stats().KeyN
		deleteCount := logCount - e.Opts.MaxBufferedLogs

		if deleteCount <= 0 {
			// Limit not exceeded
			return nil
		}

		level.Info(e.Opts.Logger).Log(
			"msg",
			fmt.Sprintf("Buffered logs limit (%d) exceeded. Purging %d logs.",
				e.Opts.MaxBufferedLogs,
				deleteCount,
			),
		)

		c := b.Cursor()
		k, _ := c.First()
		for total := 0; k != nil && total < deleteCount; total++ {
			c.Delete() // Note: This advances the cursor
			k, _ = c.First()
		}

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "deleting overflowed logs")
	}
	return nil
}

// LogString will publish a status/result log from osquery to the server. In
// the future it should buffer logs and send them at intervals.
func (e *Extension) LogString(ctx context.Context, typ logger.LogType, logText string) error {
	bucketName, err := bucketNameFromLogType(typ)
	if err != nil {
		level.Info(e.Opts.Logger).Log(
			"msg",
			fmt.Sprintf("Ignoring unknown log type: %v", typ),
		)
	}

	// Buffer the log for sending later in a batch
	err = e.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))

		key, err := b.NextSequence()
		if err != nil {
			return errors.Wrap(err, "generating key")
		}

		return b.Put(byteKeyFromUint64(key), []byte(logText))
	})

	if err != nil {
		return errors.Wrap(err, "buffering log")
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
		_, invalid, err := e.Enroll(ctx)
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
		_, invalid, err := e.Enroll(ctx)
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

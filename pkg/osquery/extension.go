package osquery

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/mixer/clock"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

	osqueryClient Querier
	initialRunner *initialRunner
}

// SetQuerier sets an osquery client on the extension, allowing
// the extension to query the running osqueryd instance.
func (e *Extension) SetQuerier(client Querier) {
	e.osqueryClient = client
	if e.initialRunner != nil {
		e.initialRunner.client = client
	}
}

// Querier allows querying osquery.
type Querier interface {
	Query(sql string) ([]map[string]string, error)
}

const (
	// Bucket name to use for launcher configuration.
	configBucket         = "config"
	initialResultsBucket = "initial_results"
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

	// Default maximum number of bytes per batch (used if not specified in
	// options). This 2MB limit is chosen based on the default grpc-go
	// limit specified in https://github.com/grpc/grpc-go/blob/master/server.go#L51
	// which is 4MB. We use 2MB to be conservative.
	defaultMaxBytesPerBatch = 2 << 20
	// Default logging interval (used if not specified in
	// options)
	defaultLoggingInterval = 60 * time.Second
	// Default maximum number of logs to buffer before purging oldest logs
	// (applies per log type).
	defaultMaxBufferedLogs = 500000
)

// ExtensionOpts is options to be passed in NewExtension
type ExtensionOpts struct {
	// EnrollSecret is the (mandatory) enroll secret used for
	// enrolling with the server.
	EnrollSecret string
	// MaxBytesPerBatch is the maximum number of bytes that should be sent in
	// one batch logging request. Any log larger than this will be dropped.
	MaxBytesPerBatch int
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
	var bucketNames = []string{configBucket, statusLogsBucket, resultLogsBucket, initialResultsBucket}

	if opts.EnrollSecret == "" {
		return nil, errors.New("empty enroll secret")
	}

	if opts.MaxBytesPerBatch == 0 {
		opts.MaxBytesPerBatch = defaultMaxBytesPerBatch
	}

	if opts.LoggingInterval == 0 {
		opts.LoggingInterval = defaultLoggingInterval
	}

	if opts.Clock == nil {
		opts.Clock = clock.DefaultClock{}
	}

	if opts.Logger == nil {
		// Nop logger
		opts.Logger = log.NewNopLogger()
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

	identifier, err := IdentifierFromDB(db)
	if err != nil {
		return nil, errors.Wrap(err, "get host identifier from db when creating new extension")
	}
	initialRunner := &initialRunner{identifier: identifier, db: db}

	return &Extension{
		logger:        opts.Logger,
		serviceClient: client,
		db:            db,
		Opts:          opts,
		done:          make(chan struct{}),
		initialRunner: initialRunner,
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
	return IdentifierFromDB(e.db)
}

// IdentifierFromDB returns the built-in launcher identifier from the config bucket.
// The function is exported to allow for building the kolide_launcher_identifier table.
func IdentifierFromDB(db *bolt.DB) (string, error) {
	var identifier string
	err := db.Update(func(tx *bolt.Tx) error {
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

// NodeKeyFromDB returns the device node key from a local bolt DB
func NodeKeyFromDB(db *bolt.DB) (string, error) {
	if db == nil {
		return "", errors.New("received a nil db")
	}

	var key []byte
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(configBucket))
		key = b.Get([]byte(nodeKeyKey))
		return nil
	})
	if err != nil {
		return "", errors.Wrap(err, "error reading node key from db")
	}
	if key != nil {
		return string(key), nil
	} else {
		return "", nil
	}
}

func isNodeInvalidErr(err error) bool {
	err = errors.Cause(err)
	if se, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
		return se.GRPCStatus().Code() == codes.Unauthenticated
	} else {
		return false
	}
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
	key, err := NodeKeyFromDB(e.db)
	if err != nil {
		return "", false, errors.Wrap(err, "error reading node key from db")
	}

	if key != "" {
		e.NodeKey = key
		return e.NodeKey, false, nil
	}

	identifier, err := e.getHostIdentifier()
	if err != nil {
		return "", true, errors.Wrap(err, "generating UUID")
	}

	enrollDetails, err := getEnrollDetails(e.osqueryClient)
	if err != nil {
		return "", true, errors.Wrap(err, "query enrollment details")
	}

	// If no cached node key, enroll for new node key
	keyString, invalid, err := e.serviceClient.RequestEnrollment(ctx, e.Opts.EnrollSecret, identifier, enrollDetails)
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
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
		level.Debug(e.logger).Log(
			"msg", "generating configs with reenroll failed",
			"err", err,
		)
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

// TODO: https://github.com/kolide/launcher/issues/366
var reenrollmentInvalidErr = errors.New("enrollment invalid, reenrollment invalid")

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) generateConfigsWithReenroll(ctx context.Context, reenroll bool) (string, error) {
	config, invalid, err := e.serviceClient.RequestConfig(ctx, e.NodeKey)
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
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
			return "", reenrollmentInvalidErr
		}

		// Don't attempt reenroll after first attempt
		return e.generateConfigsWithReenroll(ctx, false)
	}

	if err := e.initialRunner.Execute(config, e.LogString); err != nil {
		return "", errors.Wrap(err, "initial run results")
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
// Opts.MaxBytesPerBatch bytes in one run. If the logs write successfully, they
// will be deleted from the buffer. After writing (whether success or failure),
// logs over the maximum count will be purged to avoid unbounded growth of the
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
// Opts.MaxBytesPerBatch bytes worth of logs in one run. If the logs write
// successfully, they will be deleted from the buffer.
func (e *Extension) writeBufferedLogsForType(typ logger.LogType) error {
	bucketName, err := bucketNameFromLogType(typ)
	if err != nil {
		return err
	}

	// Collect up logs to be sent
	var logs []string
	var logIDs [][]byte
	err = e.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))

		c := b.Cursor()
		k, v := c.First()
		for totalBytes := 0; k != nil; {
			if len(v) > e.Opts.MaxBytesPerBatch {
				// Discard logs that are too big
				level.Info(e.Opts.Logger).Log(
					"msg", "dropped log",
					"size", len(v),
					"limit", e.Opts.MaxBytesPerBatch,
				)
			} else if totalBytes+len(v) > e.Opts.MaxBytesPerBatch {
				// Buffer is filled
				break
			} else {
				logs = append(logs, string(v))
				totalBytes += len(v)
				logIDs = append(logIDs, k)
			}
			k, v = c.Next()
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "reading buffered logs")
	}

	if len(logs) == 0 {
		// Nothing to send
		return nil
	}

	err = e.writeLogsWithReenroll(context.Background(), typ, logs, true)
	if err != nil {
		return errors.Wrap(err, "writing logs")
	}

	// Delete logs that were successfully sent
	err = e.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		for _, k := range logIDs {
			b.Delete(k)
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "deleting sent logs")
	}

	return nil
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) writeLogsWithReenroll(ctx context.Context, typ logger.LogType, logs []string, reenroll bool) error {
	_, _, invalid, err := e.serviceClient.PublishLogs(ctx, e.NodeKey, typ, logs)
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
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

// purgeBufferedLogsForType flushes the log buffers for the provided type,
// ensuring that at most Opts.MaxBufferedLogs logs remain.
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
			"msg", "Buffered logs limit exceeded. Purging excess.",
			"limit", e.Opts.MaxBufferedLogs,
			"purge_count", deleteCount,
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

// LogString will buffer logs from osquery into the local BoltDB store. No
// immediate action is taken to push the logs to the server (that is handled by
// the log publishing thread).
func (e *Extension) LogString(ctx context.Context, typ logger.LogType, logText string) error {
	if typ == logger.LogTypeInit {
		// osquery seems to send an empty init log whenever it starts
		// up. Ignore this log without generating an error.
		return nil
	}

	bucketName, err := bucketNameFromLogType(typ)
	if err != nil {
		level.Info(e.Opts.Logger).Log(
			"msg", "Received unknown log type",
			"log_type", typ,
		)
		return errors.Wrap(err, "unknown log type")
	}

	// Buffer the log for sending later in a batch
	err = e.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))

		// Log keys are generated with the auto-incrementing sequence
		// number provided by BoltDB. These must be converted to []byte
		// (which we do with byteKeyFromUint64 function).
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
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
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
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
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

func getEnrollDetails(client Querier) (service.EnrollmentDetails, error) {
	query := `
	SELECT
		osquery_info.version as osquery_version,
		os_version.build as os_build,
		os_version.name as os_name,
		os_version.platform as os_platform,
		os_version.platform_like as os_platform_like,
		os_version.version as os_version,
		system_info.hardware_model,
		system_info.hardware_serial,
		system_info.hardware_vendor,
		system_info.hostname
	FROM
		os_version,
		system_info,
		osquery_info;
`
	var details service.EnrollmentDetails
	resp, err := client.Query(query)
	if err != nil {
		return details, errors.Wrap(err, "query enrollment details")
	}

	if len(resp) < 1 {
		return details, errors.New("expected at least one row from the enrollment details query")
	}

	if val, ok := resp[0]["os_version"]; ok {
		details.OSVersion = val
	}
	if val, ok := resp[0]["os_build"]; ok {
		details.OSBuildID = val
	}
	if val, ok := resp[0]["os_name"]; ok {
		details.OSName = val
	}
	if val, ok := resp[0]["os_platform"]; ok {
		details.OSPlatform = val
	}
	if val, ok := resp[0]["os_platform_like"]; ok {
		details.OSPlatformLike = val
	}
	if val, ok := resp[0]["osquery_version"]; ok {
		details.OsqueryVersion = val
	}
	if val, ok := resp[0]["hardware_model"]; ok {
		details.HardwareModel = val
	}
	if val, ok := resp[0]["hardware_serial"]; ok {
		details.HardwareSerial = val
	}
	if val, ok := resp[0]["hardware_vendor"]; ok {
		details.HardwareVendor = val
	}
	if val, ok := resp[0]["hostname"]; ok {
		details.Hostname = val
	}

	// Using the version field from the binary.
	// The extension uses the same value to build the table, but the query runs before the extension tables are registered.
	details.LauncherVersion = version.Version().Version

	return details, nil
}

type initialRunner struct {
	identifier string
	client     Querier
	db         *bolt.DB
}

func (i *initialRunner) Execute(configBlob string, writeFn func(ctx context.Context, l logger.LogType, s string) error) error {
	var config OsqueryConfig
	if err := json.Unmarshal([]byte(configBlob), &config); err != nil {
		return errors.Wrap(err, "unmarshal osquery config blob")
	}

	var allQueries []string
	for packName, pack := range config.Packs {
		for query, _ := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			allQueries = append(allQueries, queryName)
		}
	}

	toRun, err := i.queriesToRun(allQueries)
	if err != nil {
		return errors.Wrap(err, "checking if query should run")
	}

	var initialRunResults []OsqueryResultLog
	for packName, pack := range config.Packs {
		for query, queryContent := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			if _, ok := toRun[queryName]; !ok {
				continue
			}
			resp, err := i.client.Query(queryContent.Query)
			if err != nil {
				return errors.Wrapf(err, "query for initial run results %s", queryName)
			}
			initialRunResults = append(initialRunResults, OsqueryResultLog{
				Name:           queryName,
				HostIdentifier: i.identifier,
				UnixTime:       int(time.Now().UTC().Unix()),
				DiffResults:    &DiffResults{Added: resp},
				Epoch:          0,
			})
		}
	}

	for _, result := range initialRunResults {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(result); err != nil {
			return errors.Wrap(err, "encoding initial run result")
		}
		if err := writeFn(context.Background(), logger.LogTypeString, buf.String()); err != nil {
			return errors.Wrap(err, "writing encoded initial result log")
		}
	}

	if err := i.cacheRanQueries(toRun); err != nil {
		return err
	}

	return nil
}

func (i *initialRunner) queriesToRun(allFromConfig []string) (map[string]struct{}, error) {
	known := make(map[string]struct{})
	err := i.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(initialResultsBucket))
		for _, q := range allFromConfig {
			knownQuery := b.Get([]byte(q))
			if knownQuery != nil {
				continue
			}
			known[q] = struct{}{}
		}
		return nil
	})

	return known, errors.Wrap(err, "check bolt for queries to run")
}

func (i *initialRunner) cacheRanQueries(known map[string]struct{}) error {
	err := i.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(initialResultsBucket))
		for q := range known {
			if err := b.Put([]byte(q), []byte(q)); err != nil {
				return errors.Wrapf(err, "cache initial result query %q", q)
			}
		}
		return nil
	})
	return errors.Wrap(err, "caching known initial result queries")
}

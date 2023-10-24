package osquery

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"sort"

	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/mixer/clock"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"
	"github.com/pkg/errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Extension is the implementation of the osquery extension
// methods. It acts as a communication intermediary between osquery
// and servers -- It provides a grpc and jsonrpc interface for
// osquery. It does not provide any tables.
type Extension struct {
	NodeKey       string
	Opts          ExtensionOpts
	knapsack      types.Knapsack
	serviceClient service.KolideService
	enrollMutex   sync.Mutex
	done          chan struct{}
	interrupted   bool
	wg            sync.WaitGroup
	logger        log.Logger

	initialRunner *initialRunner
}

// SetQuerier sets an osquery client on the extension, allowing
// the extension to query the running osqueryd instance.
func (e *Extension) SetQuerier(client Querier) {
	if e.initialRunner != nil {
		e.initialRunner.client = client
	}
}

// Querier allows querying osquery.
type Querier interface {
	Query(sql string) ([]map[string]string, error)
}

const (
	// DB key for UUID
	uuidKey = "uuid"
	// DB key for node key
	nodeKeyKey = "nodeKey"
	// DB key for last retrieved config
	configKey = "config"
	// DB keys for the rsa keys
	privateKeyKey = "privateKey"

	// Old things to delete
	xPublicKeyKey      = "publicKey"
	xKeyFingerprintKey = "keyFingerprint"

	// Default maximum number of bytes per batch (used if not specified in
	// options). This 3MB limit is chosen based on the default grpc-go
	// limit specified in https://github.com/grpc/grpc-go/blob/master/server.go#L51
	// which is 4MB. We use 3MB to be conservative.
	defaultMaxBytesPerBatch = 3 << 20
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
	// RunDifferentialQueriesImmediately allows the client to execute a new query the first time it sees it,
	// bypassing the scheduler.
	RunDifferentialQueriesImmediately bool
}

// NewExtension creates a new Extension from the provided service.KolideService
// implementation. The background routines should be started by calling
// Start().
func NewExtension(client service.KolideService, k types.Knapsack, opts ExtensionOpts) (*Extension, error) {
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

	configStore := k.ConfigStore()

	if err := SetupLauncherKeys(configStore); err != nil {
		return nil, fmt.Errorf("setting up initial launcher keys: %w", err)
	}

	if err := agent.SetupKeys(opts.Logger, configStore); err != nil {
		return nil, fmt.Errorf("setting up agent keys: %w", err)
	}

	identifier, err := IdentifierFromDB(configStore)
	if err != nil {
		return nil, fmt.Errorf("get host identifier from db when creating new extension: %w", err)
	}

	nodekey, err := NodeKey(configStore)
	if err != nil {
		level.Debug(opts.Logger).Log("msg", "NewExtension got error reading nodekey. Ignoring", "err", err)
		return nil, fmt.Errorf("reading nodekey from db: %w", err)
	} else if nodekey == "" {
		level.Debug(opts.Logger).Log("msg", "NewExtension did not find a nodekey. Likely first enroll")
	} else {
		level.Debug(opts.Logger).Log("msg", "NewExtension found existing nodekey")
	}

	initialRunner := &initialRunner{
		logger:     log.With(opts.Logger, "component", "initial_runner"),
		identifier: identifier,
		store:      k.InitialResultsStore(),
		enabled:    opts.RunDifferentialQueriesImmediately,
	}

	return &Extension{
		logger:        log.With(opts.Logger, "component", "osquery_extension"),
		serviceClient: client,
		knapsack:      k,
		NodeKey:       nodekey,
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
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if e.interrupted {
		return
	}
	e.interrupted = true

	close(e.done)
	e.wg.Wait()
}

// getHostIdentifier returns the UUID identifier associated with this host. If
// there is an existing identifier, that should be returned. If not, the
// identifier should be randomly generated and persisted.
func (e *Extension) getHostIdentifier() (string, error) {
	return IdentifierFromDB(e.knapsack.ConfigStore())
}

// SetupLauncherKeys configures the various keys used for communication.
//
// There are 3 keys:
// 1. The RSA key. This is stored in the launcher DB, and was the first key used by krypto. We are deprecating it.
// 2. The hardware keys -- these are in the secure enclave (TPM or Apple's thing) These are used to identify the device
// 3. The launcher install key -- this is an ECC key that is sometimes used in conjunction with (2)
func SetupLauncherKeys(configStore types.KVStore) error {
	// Soon-to-be-deprecated RSA keys
	if err := ensureRsaKey(configStore); err != nil {
		return fmt.Errorf("ensuring rsa key: %w", err)
	}

	// Remove things we don't keep in the bucket any more
	for _, k := range []string{xPublicKeyKey, xKeyFingerprintKey} {
		if err := configStore.Delete([]byte(k)); err != nil {
			return fmt.Errorf("deleting %s: %w", k, err)
		}
	}

	return nil
}

// ensureRsaKey will create an RSA key in the launcher DB if one does not already exist. This is the old key that krypto used. We are moving away from it.
func ensureRsaKey(configStore types.GetterSetter) error {
	// If it exists, we're good
	_, err := configStore.Get([]byte(privateKeyKey))
	if err != nil {
		return nil
	}

	// Create a random key
	key, err := rsaRandomKey()
	if err != nil {
		return fmt.Errorf("generating private key: %w", err)
	}

	keyDer, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshalling private key: %w", err)
	}

	if err := configStore.Set([]byte(privateKeyKey), keyDer); err != nil {
		return fmt.Errorf("storing private key: %w", err)
	}

	return nil
}

// PrivateRSAKeyFromDB returns the private launcher key. This is the old key used to authenticate various launcher communications.
func PrivateRSAKeyFromDB(configStore types.Getter) (*rsa.PrivateKey, error) {
	privateKey, err := configStore.Get([]byte(privateKeyKey))
	if err != nil {
		return nil, fmt.Errorf("error reading private key info from db: %w", err)
	}

	key, err := x509.ParsePKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("error parsing private key: %w", err)
	}

	rsakey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("Private key is not an rsa key")
	}

	return rsakey, nil
}

// PublicRSAKeyFromDB returns the public portions of the launcher key. This is exposed in various launcher info structures.
func PublicRSAKeyFromDB(configStore types.Getter) (string, string, error) {
	privateKey, err := PrivateRSAKeyFromDB(configStore)
	if err != nil {
		return "", "", fmt.Errorf("reading private key: %w", err)
	}

	fingerprint, err := rsaFingerprint(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("generating fingerprint: %w", err)
	}

	var publicKey bytes.Buffer
	if err := RsaPrivateKeyToPem(privateKey, &publicKey); err != nil {
		return "", "", fmt.Errorf("marshalling pub: %w", err)
	}

	return publicKey.String(), fingerprint, nil
}

// IdentifierFromDB returns the built-in launcher identifier from the config bucket.
// The function is exported to allow for building the kolide_launcher_info table.
func IdentifierFromDB(configStore types.GetterSetter) (string, error) {
	var identifier string
	uuidBytes, _ := configStore.Get([]byte(uuidKey))
	gotID, err := uuid.ParseBytes(uuidBytes)

	// Use existing UUID
	if err == nil {
		identifier = gotID.String()
		return identifier, nil
	}

	// Generate new (random) UUID
	gotID, err = uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generating new UUID: %w", err)
	}
	identifier = gotID.String()

	// Save new UUID
	err = configStore.Set([]byte(uuidKey), []byte(identifier))
	if err != nil {
		return "", fmt.Errorf("saving new UUID: %w", err)
	}

	return identifier, nil
}

// NodeKey returns the device node key from the storage layer
func NodeKey(getter types.Getter) (string, error) {
	key, err := getter.Get([]byte(nodeKeyKey))
	if err != nil {
		return "", fmt.Errorf("error getting node key: %w", err)
	}
	if key != nil {
		return string(key), nil
	} else {
		return "", nil
	}
}

// Config returns the device config from the storage layer
func Config(getter types.Getter) (string, error) {
	key, err := getter.Get([]byte(configKey))
	if err != nil {
		return "", fmt.Errorf("error getting config key: %w", err)
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
	logger := log.With(e.logger, "method", "enroll")

	level.Debug(logger).Log("msg", "checking enrollment")

	// Only one thread should ever be allowed to attempt enrollment at the
	// same time.
	e.enrollMutex.Lock()
	defer e.enrollMutex.Unlock()

	// If we already have a successful enrollment (perhaps from another
	// thread), no need to do anything else.
	if e.NodeKey != "" {
		level.Debug(logger).Log("msg", "node key exists, skipping enrollment")
		return e.NodeKey, false, nil
	}

	// Look up a node key cached in the local store
	key, err := NodeKey(e.knapsack.ConfigStore())
	if err != nil {
		return "", false, fmt.Errorf("error reading node key from db: %w", err)
	}

	if key != "" {
		level.Debug(logger).Log("msg", "found stored node key, skipping enrollment")
		e.NodeKey = key
		return e.NodeKey, false, nil
	}

	level.Debug(logger).Log("msg", "starting enrollment")

	identifier, err := e.getHostIdentifier()
	if err != nil {
		return "", true, fmt.Errorf("generating UUID: %w", err)
	}

	// We used to see the enrollment details fail, but now that we're running as an exec,
	// it seems less likely. Try a couple times, but backoff fast.
	var enrollDetails service.EnrollmentDetails
	if osqPath := e.knapsack.LatestOsquerydPath(ctx); osqPath == "" {
		level.Info(logger).Log("msg", "Cannot get additional enrollment details without an osqueryd path. This is probably CI")
	} else {
		if err := backoff.WaitFor(func() error {
			enrollDetails, err = getEnrollDetails(ctx, osqPath)
			if err != nil {
				level.Debug(logger).Log("msg", "getEnrollDetails failed in backoff", "err", err)
			}
			return err
		}, 30*time.Second, 5*time.Second); err != nil {
			if os.Getenv("LAUNCHER_DEBUG_ENROLL_DETAILS_REQUIRED") == "true" {
				return "", true, fmt.Errorf("query enrollment details: %w", err)
			}

			level.Info(logger).Log("msg", "Failed to get enrollment details (even with retries). Moving on", "err", err)
		}
	}
	// If no cached node key, enroll for new node key
	// note that we set invalid two ways. Via the return, _or_ via isNodeInvaliderr
	keyString, invalid, err := e.serviceClient.RequestEnrollment(ctx, e.Opts.EnrollSecret, identifier, enrollDetails)
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
		return "", true, fmt.Errorf("transport error in enrollment: %w", err)
	}
	if invalid {
		if err == nil {
			err = errors.New("no further error")
		}
		return "", true, fmt.Errorf("enrollment invalid: %w", err)
	}

	// Save newly acquired node key if successful
	err = e.knapsack.ConfigStore().Set([]byte(nodeKeyKey), []byte(keyString))
	if err != nil {
		return "", true, fmt.Errorf("saving node key: %w", err)
	}

	e.NodeKey = keyString

	level.Debug(logger).Log("msg", "completed enrollment")

	return e.NodeKey, false, nil
}

// RequireReenroll clears the existing node key information, ensuring that the
// next call to Enroll will cause the enrollment process to take place.
func (e *Extension) RequireReenroll(ctx context.Context) {
	e.enrollMutex.Lock()
	defer e.enrollMutex.Unlock()
	// Clear the node key such that reenrollment is required.
	e.NodeKey = ""
	e.knapsack.ConfigStore().Delete([]byte(nodeKeyKey))
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
		confBytes, _ = e.knapsack.ConfigStore().Get([]byte(configKey))

		if len(confBytes) == 0 {
			return nil, fmt.Errorf("loading config failed, no cached config: %w", err)
		}
		config = string(confBytes)
	} else {
		// Store good config
		e.knapsack.ConfigStore().Set([]byte(configKey), []byte(config))
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
		return "", fmt.Errorf("transport error retrieving config: %w", err)
	}

	if invalid {
		if err == nil {
			err = errors.New("no further error")
		}

		if !reenroll {
			return "", fmt.Errorf("enrollment invalid, reenroll disabled: %w", err)
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx)
		if err != nil {
			return "", fmt.Errorf("enrollment invalid, reenrollment errored: %w", err)
		}
		if invalid {
			return "", reenrollmentInvalidErr
		}

		// Don't attempt reenroll after first attempt
		return e.generateConfigsWithReenroll(ctx, false)
	}

	// If osquery has been running successfully for 10 minutes, then turn off verbose logs.
	osqueryVerbose := true
	if uptimeMins, err := history.LatestInstanceUptimeMinutes(); err == nil && uptimeMins >= 10 {
		// Only log the state change once -- RequestConfig happens every 5 mins
		if uptimeMins <= 15 {
			level.Debug(e.logger).Log("msg", "osquery has been up for more than 10 minutes, turning off verbose logging", "uptime_mins", uptimeMins)
		}
		osqueryVerbose = false
	}
	config = e.setVerbose(config, osqueryVerbose)

	if err := e.initialRunner.Execute(config, e.writeLogsWithReenroll); err != nil {
		return "", fmt.Errorf("initial run results: %w", err)
	}

	return config, nil
}

// setVerbose modifies the given config to add the `verbose` option.
func (e *Extension) setVerbose(config string, osqueryVerbose bool) string {
	var cfg map[string]any

	if config != "" {
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			level.Debug(e.logger).Log("msg", "could not unmarshal config, cannot set verbose", "err", err)
			return config
		}
	} else {
		cfg = make(map[string]any)
	}

	var opts map[string]any
	if cfgOpts, ok := cfg["options"]; ok {
		opts, ok = cfgOpts.(map[string]any)
		if !ok {
			level.Debug(e.logger).Log("msg", "config options are malformed, cannot set verbose")
			return config
		}
	} else {
		opts = make(map[string]any)
	}

	opts["verbose"] = osqueryVerbose
	cfg["options"] = opts

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		level.Debug(e.logger).Log("msg", "could not marshal config, cannot set verbose", "err", err)
		return config
	}
	return string(cfgBytes)
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

// storeForLogType returns the store with the logs of the provided type.
func storeForLogType(s types.Stores, typ logger.LogType) (types.KVStore, error) {
	switch typ {
	case logger.LogTypeString, logger.LogTypeSnapshot:
		return s.ResultLogsStore(), nil
	case logger.LogTypeStatus:
		return s.StatusLogsStore(), nil
	default:
		return nil, fmt.Errorf("unknown log type: %v", typ)

	}
}

func sortStoreKeys(s types.KVStore) ([][]byte, error) {
	// create an array of all the keys
	var keys [][]byte
	err := s.ForEach(func(k, _ []byte) error {
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterating keys: %w", err)
	}

	// sort the keys
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i], keys[j]) < 0
	})

	return keys, nil
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
				"err", fmt.Errorf("sending %v logs: %w", typ, err),
			)
		}

		// Purge overflow
		err = e.purgeBufferedLogsForType(typ)
		if err != nil {
			level.Info(e.Opts.Logger).Log(
				"err", fmt.Errorf("purging %v logs: %w", typ, err),
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
	store, err := storeForLogType(e.knapsack, typ)
	if err != nil {
		return err
	}

	// Collect up logs to be sent
	var logs []string
	var totalBytes int

	keys, err := sortStoreKeys(store)
	if err != nil {
		return fmt.Errorf("getting buffered log keys: %w", err)
	}

	lastKeyIndex := 0
	for _, k := range keys {
		lastKeyIndex++

		v, err := store.Get(k)
		if err != nil {
			level.Error(e.Opts.Logger).Log(
				"msg", "error getting buffered log entry",
				"err", err,
			)
			continue
		}

		if len(v) > e.Opts.MaxBytesPerBatch {
			// Discard logs that are too big
			logheadSize := minInt(len(v), 100)
			level.Info(e.Opts.Logger).Log(
				"msg", "dropped log",
				"logID", k,
				"size", len(v),
				"limit", e.Opts.MaxBytesPerBatch,
				"loghead", string(v)[0:logheadSize],
			)
			continue
		}

		if totalBytes+len(v) > e.Opts.MaxBytesPerBatch {
			lastKeyIndex--
			break
		}

		totalBytes += len(v)

		// add to logs to be sent
		logs = append(logs, string(v))
	}

	if len(logs) == 0 {
		// Nothing to send
		return nil
	}

	if err = e.writeLogsWithReenroll(context.Background(), typ, logs, true); err != nil {
		return fmt.Errorf("writing logs: %w", err)
	}

	if err = store.Delete(keys[:lastKeyIndex]...); err != nil {
		return fmt.Errorf("deleting sent logs: %w", err)
	}

	return nil
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) writeLogsWithReenroll(ctx context.Context, typ logger.LogType, logs []string, reenroll bool) error {
	_, _, invalid, err := e.serviceClient.PublishLogs(ctx, e.NodeKey, typ, logs)
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
		return fmt.Errorf("transport error sending logs: %w", err)
	}

	if invalid {
		if !reenroll {
			return errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx)
		if err != nil {
			return fmt.Errorf("enrollment invalid, reenrollment errored: %w", err)
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
	store, err := storeForLogType(e.knapsack, typ)
	if err != nil {
		return err
	}

	keys, err := sortStoreKeys(store)
	if err != nil {
		return fmt.Errorf("getting buffered log keys: %w", err)
	}

	deleteCount := len(keys) - e.Opts.MaxBufferedLogs

	if deleteCount <= 0 {
		// Limit not exceeded
		return nil
	}

	level.Info(e.Opts.Logger).Log(
		"msg", "Buffered logs limit exceeded. Purging excess.",
		"limit", e.Opts.MaxBufferedLogs,
		"purge_count", len(keys),
	)

	if err := store.Delete(keys[:deleteCount]...); err != nil {
		return fmt.Errorf("deleting overflowed logs: %w", err)
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

	store, err := storeForLogType(e.knapsack, typ)
	if err != nil {
		level.Info(e.Opts.Logger).Log(
			"msg", "Received unknown log type",
			"log_type", typ,
		)
		return fmt.Errorf("unknown log type: %w", err)
	}

	keys, err := sortStoreKeys(store)
	if err != nil {
		return fmt.Errorf("getting keys for log string: %w", err)
	}

	uint64Key := uint64(0)
	if len(keys) > 0 {
		uint64Key = uint64FromByteKey(keys[len(keys)-1]) + 1
	}

	if err := store.Set(byteKeyFromUint64(uint64Key), []byte(logText)); err != nil {
		return fmt.Errorf("buffering log: %w", err)
	}

	return nil
}

// GetQueries will request the distributed queries to execute from the server.
func (e *Extension) GetQueries(ctx context.Context) (*distributed.GetQueriesResult, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	return e.getQueriesWithReenroll(ctx, true)
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) getQueriesWithReenroll(ctx context.Context, reenroll bool) (*distributed.GetQueriesResult, error) {
	// Note that we set invalid two ways -- in the return, and via isNodeinvaliderr
	queries, invalid, err := e.serviceClient.RequestQueries(ctx, e.NodeKey)
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
		return nil, fmt.Errorf("transport error getting queries: %w", err)
	}

	if invalid {
		if err == nil {
			err = errors.New("no further error")
		}

		if !reenroll {
			return nil, fmt.Errorf("enrollment invalid, reenroll disabled: %w", err)
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx)
		if err != nil {
			return nil, fmt.Errorf("enrollment invalid, reenrollment errored: %w", err)
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
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	return e.writeResultsWithReenroll(ctx, results, true)
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) writeResultsWithReenroll(ctx context.Context, results []distributed.Result, reenroll bool) error {
	_, _, invalid, err := e.serviceClient.PublishResults(ctx, e.NodeKey, results)
	if isNodeInvalidErr(err) {
		invalid = true
	} else if err != nil {
		return fmt.Errorf("transport error writing results: %w", err)
	}

	if invalid {
		if !reenroll {
			return errors.New("enrollment invalid, reenroll disabled")
		}

		e.RequireReenroll(ctx)
		_, invalid, err := e.Enroll(ctx)
		if err != nil {
			return fmt.Errorf("enrollment invalid, reenrollment errored: %w", err)
		}
		if invalid {
			return errors.New("enrollment invalid, reenrollment invalid")
		}

		// Don't attempt reenroll after first attempt
		return e.writeResultsWithReenroll(ctx, results, false)
	}

	return nil
}

type initialRunner struct {
	logger     log.Logger
	enabled    bool
	identifier string
	client     Querier
	store      types.GetterSetter
}

func (i *initialRunner) Execute(configBlob string, writeFn func(ctx context.Context, l logger.LogType, results []string, reeenroll bool) error) error {
	var config OsqueryConfig
	if err := json.Unmarshal([]byte(configBlob), &config); err != nil {
		return fmt.Errorf("unmarshal osquery config blob: %w", err)
	}

	var allQueries []string
	for packName, pack := range config.Packs {
		// only run queries from kolide packs
		if !strings.Contains(packName, "_kolide_") {
			continue
		}

		// Run all the queries, snapshot and differential
		for query := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			allQueries = append(allQueries, queryName)
		}
	}

	toRun, err := i.queriesToRun(allQueries)
	if err != nil {
		return fmt.Errorf("checking if query should run: %w", err)
	}

	var initialRunResults []OsqueryResultLog
	for packName, pack := range config.Packs {
		if !i.enabled { // only execute them when the plugin is enabled.
			break
		}
		for query, queryContent := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			if _, ok := toRun[queryName]; !ok {
				continue
			}
			resp, err := i.client.Query(queryContent.Query)
			// returning here causes the rest of the queries not to run
			// this is a bummer because often configs have queries with bad syntax/tables that do not exist.
			// log the error and move on.
			// using debug to not fill disks. the worst that will happen is that the result will come in later.
			level.Debug(i.logger).Log(
				"msg", "querying for initial results",
				"query_name", queryName,
				"err", err,
				"results", len(resp),
			)
			if err != nil || len(resp) == 0 {
				continue
			}

			initialRunResults = append(initialRunResults, OsqueryResultLog{
				Name:           queryName,
				HostIdentifier: i.identifier,
				UnixTime:       int(time.Now().UTC().Unix()),
				DiffResults:    &DiffResults{Added: resp},
			})
		}
	}

	cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, result := range initialRunResults {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(result); err != nil {
			return fmt.Errorf("encoding initial run result: %w", err)
		}
		if err := writeFn(cctx, logger.LogTypeString, []string{buf.String()}, true); err != nil {
			level.Debug(i.logger).Log(
				"msg", "writing initial result log to server",
				"query_name", result.Name,
				"err", err,
			)
			continue
		}
	}

	// note: caching would happen always on first use, even if the runner is not enabled.
	// This avoids the problem of queries not being known even though they've been in the config for a long time.
	if err := i.cacheRanQueries(toRun); err != nil {
		return err
	}

	return nil
}

func (i *initialRunner) queriesToRun(allFromConfig []string) (map[string]struct{}, error) {
	known := make(map[string]struct{})

	for _, q := range allFromConfig {
		knownQuery, err := i.store.Get([]byte(q))
		if err != nil {
			return nil, fmt.Errorf("check store for queries to run: %w", err)
		}
		if knownQuery != nil {
			continue
		}
		known[q] = struct{}{}
	}

	return known, nil
}

func (i *initialRunner) cacheRanQueries(known map[string]struct{}) error {
	for q := range known {
		if err := i.store.Set([]byte(q), []byte(q)); err != nil {
			return fmt.Errorf("cache initial result query %q: %w", q, err)
		}
	}

	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}

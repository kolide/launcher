package osquery

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/uninstall"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/service"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"
	"github.com/pkg/errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// settingsStoreWriter writes to our startup settings store
type settingsStoreWriter interface {
	WriteSettings() error
}

// Extension is the implementation of the osquery extension
// methods. It acts as a communication intermediary between osquery
// and servers -- It provides a grpc and jsonrpc interface for
// osquery. It does not provide any tables.
type Extension struct {
	NodeKey                       string
	Opts                          ExtensionOpts
	registrationId                string
	knapsack                      types.Knapsack
	serviceClient                 service.KolideService
	settingsWriter                settingsStoreWriter
	enrollMutex                   *sync.Mutex
	done                          chan struct{}
	interrupted                   atomic.Bool
	slogger                       *slog.Logger
	logPublicationState           *logPublicationState
	lastRequestQueriesTimestamp   *atomic.Int64
	distributedForwardingInterval *atomic.Int64 // how frequently to forward RequestQueries requests to the cloud, in seconds
	forwardAllDistributedUntil    *atomic.Int64 // allows for accelerated distributed requests until given timestamp
}

const (
	// DB key for UUID
	uuidKey = "uuid"
	// DB key for node key
	nodeKeyKey = "nodeKey"
	// DB key for last retrieved config
	configKey = "config"

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

	// How frequently osquery should check for distributed queries to run.
	// We set this to 5 seconds, which is more frequent than we think the
	// server can comfortably handle, so we only forward these requests
	// to the cloud once every 60 seconds.
	osqueryDistributedInterval = 5
)

var (
	// Osquery configuration options that we set during the osquery instance's startup period.
	// These are the override, non-standard values.
	startupOsqueryConfigOptions = map[string]any{
		"verbose":              true, // receive as many osquery logs as we can, in case we need to troubleshoot an issue
		"distributed_interval": osqueryDistributedInterval,
	}

	// Osquery configuration options that we set after the osquery instance has been up and running
	// for at least 10 minutes. These are the "normal" values. We continue to override the distributed
	// interval to 5 seconds.
	postStartupOsqueryConfigOptions = map[string]any{
		"verbose":              false,
		"distributed_interval": osqueryDistributedInterval,
	}
)

// ExtensionOpts is options to be passed in NewExtension
type ExtensionOpts struct {
	// MaxBytesPerBatch is the maximum number of bytes that should be sent in
	// one batch logging request. Any log larger than this will be dropped.
	MaxBytesPerBatch int
	// LoggingInterval is the interval at which logs should be flushed to
	// the server.
	LoggingInterval time.Duration
	// MaxBufferedLogs is the maximum number of logs to buffer before
	// purging oldest logs (applies per log type).
	MaxBufferedLogs int
	// RunDifferentialQueriesImmediately allows the client to execute a new query the first time it sees it,
	// bypassing the scheduler.
	RunDifferentialQueriesImmediately bool
}

type iterationTerminatedError struct{}

func (e iterationTerminatedError) Error() string {
	return "ceasing kv store iteration"
}

// NewExtension creates a new Extension from the provided service.KolideService
// implementation. The background routines should be started by calling
// Start().
func NewExtension(ctx context.Context, client service.KolideService, settingsWriter settingsStoreWriter, k types.Knapsack, registrationId string, opts ExtensionOpts) (*Extension, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	slogger := k.Slogger().With("component", "osquery_extension", "registration_id", registrationId)

	if opts.MaxBytesPerBatch == 0 {
		opts.MaxBytesPerBatch = defaultMaxBytesPerBatch
	}

	if opts.LoggingInterval == 0 {
		opts.LoggingInterval = defaultLoggingInterval
	}

	if opts.MaxBufferedLogs == 0 {
		opts.MaxBufferedLogs = defaultMaxBufferedLogs
	}

	configStore := k.ConfigStore()

	nodekey, err := NodeKey(configStore, registrationId)
	if err != nil {
		slogger.Log(ctx, slog.LevelDebug,
			"NewExtension got error reading nodekey. Ignoring",
			"err", err,
		)
		return nil, fmt.Errorf("reading nodekey from db: %w", err)
	} else if nodekey == "" {
		slogger.Log(ctx, slog.LevelDebug,
			"NewExtension did not find a nodekey. Likely first enroll",
		)
	} else {
		slogger.Log(ctx, slog.LevelDebug,
			"NewExtension found existing nodekey",
		)
	}

	initialTimestamp := &atomic.Int64{}
	initialTimestamp.Store(0)

	distributedForwardingInterval := &atomic.Int64{}
	distributedForwardingInterval.Store(int64(k.DistributedForwardingInterval().Seconds()))

	forwardAllDistributedUntil := &atomic.Int64{}
	forwardAllDistributedUntil.Store(time.Now().Unix() + 120) // forward all queries for the first 2 minutes after startup

	e := &Extension{
		slogger:                       slogger,
		serviceClient:                 client,
		settingsWriter:                settingsWriter,
		registrationId:                registrationId,
		knapsack:                      k,
		NodeKey:                       nodekey,
		Opts:                          opts,
		enrollMutex:                   &sync.Mutex{},
		done:                          make(chan struct{}),
		logPublicationState:           NewLogPublicationState(opts.MaxBytesPerBatch),
		lastRequestQueriesTimestamp:   initialTimestamp,
		distributedForwardingInterval: distributedForwardingInterval,
		forwardAllDistributedUntil:    forwardAllDistributedUntil,
	}
	k.RegisterChangeObserver(e, keys.DistributedForwardingInterval)

	return e, nil
}

func (e *Extension) Execute() error {
	// Process logs until shutdown
	ticker := time.NewTicker(e.Opts.LoggingInterval)
	defer ticker.Stop()
	for {
		e.writeAndPurgeLogs()

		// select to either exit or write another batch of logs
		select {
		case <-e.done:
			e.slogger.Log(context.TODO(), slog.LevelInfo,
				"osquery extension received shutdown request",
			)
			return nil
		case <-ticker.C:
			// Resume loop
		}
	}
}

// Shutdown should be called to cleanup the resources and goroutines associated
// with this extension.
func (e *Extension) Shutdown(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if e.interrupted.Swap(true) {
		return
	}

	e.knapsack.DeregisterChangeObserver(e)
	close(e.done)
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface -- handles updates to flags
// that we care about, which is DistributedForwardingInterval.
func (e *Extension) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	for _, flagKey := range flagKeys {
		if flagKey == keys.DistributedForwardingInterval {
			e.distributedForwardingInterval.Store(int64(e.knapsack.DistributedForwardingInterval().Seconds()))
			// That's the only flag we care about -- we can break here
			break
		}
	}
}

// getHostIdentifier returns the UUID identifier associated with this host. If
// there is an existing identifier, that should be returned. If not, the
// identifier should be randomly generated and persisted.
func (e *Extension) getHostIdentifier() (string, error) {
	return IdentifierFromDB(e.knapsack.ConfigStore(), e.registrationId)
}

// IdentifierFromDB returns the built-in launcher identifier from the config bucket.
// The function is exported to allow for building the kolide_launcher_info table.
func IdentifierFromDB(configStore types.GetterSetter, registrationId string) (string, error) {
	var identifier string
	uuidBytes, _ := configStore.Get(storage.KeyByIdentifier([]byte(uuidKey), storage.IdentifierTypeRegistration, []byte(registrationId)))
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
	err = configStore.Set(storage.KeyByIdentifier([]byte(uuidKey), storage.IdentifierTypeRegistration, []byte(registrationId)), []byte(identifier))
	if err != nil {
		return "", fmt.Errorf("saving new UUID: %w", err)
	}

	return identifier, nil
}

// NodeKey returns the device node key from the storage layer
func NodeKey(getter types.Getter, registrationId string) (string, error) {
	key, err := getter.Get(storage.KeyByIdentifier([]byte(nodeKeyKey), storage.IdentifierTypeRegistration, []byte(registrationId)))
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
func Config(getter types.Getter, registrationId string) (string, error) {
	key, err := getter.Get(storage.KeyByIdentifier([]byte(configKey), storage.IdentifierTypeRegistration, []byte(registrationId)))
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

// addRegistration should be called after enrollment to store enrollment/registration details
// in our persistent store under e.registrationId. We frequently use the munemo associated with
// the registration (e.g. in checkups), so we parse the enrollment secret in order to extract
// the munemo from the claims and store it as well.
func (e *Extension) addRegistration(nodeKey string, enrollmentSecret string) error {
	// Extract munemo from enrollment secret.
	// We do not have the key, and thus cannot verify -- so we use ParseUnverified.
	token, _, err := new(jwt.Parser).ParseUnverified(enrollmentSecret, jwt.MapClaims{})
	if err != nil {
		return fmt.Errorf("parsing enrollment secret: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("no claims in enrollment secret")
	}
	munemo, munemoFound := claims["organization"]
	if !munemoFound {
		return errors.New("no claim for organization in enrollment secret, cannot get munemo")
	}

	if err := e.knapsack.SaveRegistration(e.registrationId, fmt.Sprintf("%s", munemo), nodeKey, enrollmentSecret); err != nil {
		return fmt.Errorf("saving registration: %w", err)
	}

	e.slogger.Log(context.TODO(), slog.LevelInfo,
		"successfully stored new registration",
		"munemo", munemo,
	)

	return nil
}

// ensureNodeKeyStored saves the provided nodeKey under the registration associated with
// e.registrationId. Since storing registrations in the store is newer functionality,
// we may not actually have a stored registration yet -- this will happen if this launcher
// install enrolled before we started storing registrations in the store. In this case,
// we fetch the enrollment secret and call `e.addRegistration` instead. ensureNodeKeyStored
// does handle the case where there is an existing registration and the node key changed,
// but we don't expect this use case at the moment. (On re-enroll, the registration is deleted,
// then re-added via addRegistration instead.)
func (e *Extension) ensureNodeKeyStored(nodeKey string) error {
	// Get the existing registration in order to update it with the new node key
	registrationStore := e.knapsack.RegistrationStore()
	existingRegistrationRaw, err := registrationStore.Get([]byte(e.registrationId))
	if err != nil {
		return fmt.Errorf("getting existing registration: %w", err)
	}

	// If the registration doesn't already exist (launcher probably enrolled before we started
	// storing registrations in the store), add it instead
	if existingRegistrationRaw == nil {
		// Grab the enroll secret, since we need that to create a new registration
		enrollSecret, err := e.knapsack.ReadEnrollSecret()
		if err != nil {
			e.slogger.Log(context.TODO(), slog.LevelWarn,
				"unable to read enroll secret to add new registration",
				"err", err,
			)
			// don't return an error here, there is nothing we can do. this is the
			// result of a bad/manual installation (or uninstallation) and any errors
			// returned here are reported
			return nil
		}
		return e.addRegistration(nodeKey, enrollSecret)
	}

	// We have an existing registration -- unmarshal it so we can update it appropriately
	var existingRegistration types.Registration
	if err := json.Unmarshal(existingRegistrationRaw, &existingRegistration); err != nil {
		return fmt.Errorf("unmarshalling existing registration: %w", err)
	}

	// Check to see if node key changed -- if not, no need to do anything here
	if existingRegistration.NodeKey == nodeKey {
		return nil
	}

	// Make update
	existingRegistration.NodeKey = nodeKey
	updatedRegistrationRaw, err := json.Marshal(existingRegistration)
	if err != nil {
		return fmt.Errorf("marshalling updated registration: %w", err)
	}

	if err := e.knapsack.RegistrationStore().Set([]byte(e.registrationId), updatedRegistrationRaw); err != nil {
		return fmt.Errorf("updating registration: %w", err)
	}

	e.slogger.Log(context.TODO(), slog.LevelInfo,
		"successfully updated registration's node key",
		"munemo", existingRegistration.Munemo,
	)
	return nil
}

// Enroll will attempt to enroll the host using the provided enroll secret for
// identification. If the host is already enrolled, the existing node key will
// be returned. To force re-enrollment, use RequireReenroll.
func (e *Extension) Enroll(ctx context.Context) (string, bool, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	e.slogger.Log(ctx, slog.LevelInfo,
		"checking enrollment",
	)
	span.AddEvent("checking_enrollment")

	// Only one thread should ever be allowed to attempt enrollment at the
	// same time.
	e.enrollMutex.Lock()
	defer e.enrollMutex.Unlock()

	// If we already have a successful enrollment (perhaps from another
	// thread), no need to do anything else.
	if e.NodeKey != "" {
		e.slogger.Log(ctx, slog.LevelDebug,
			"node key exists, skipping enrollment",
		)
		span.AddEvent("node_key_already_exists")
		if err := e.ensureNodeKeyStored(e.NodeKey); err != nil {
			e.slogger.Log(ctx, slog.LevelError,
				"could not update registration",
				"err", err,
			)
		}
		return e.NodeKey, false, nil
	}

	// Look up a node key cached in the local store
	key, err := NodeKey(e.knapsack.ConfigStore(), e.registrationId)
	if err != nil {
		observability.SetError(span, fmt.Errorf("error reading node key from db: %w", err))
		return "", false, fmt.Errorf("error reading node key from db: %w", err)
	}

	if key != "" {
		e.slogger.Log(ctx, slog.LevelDebug,
			"found stored node key, skipping enrollment",
		)
		span.AddEvent("found_stored_node_key")
		e.NodeKey = key
		if err := e.ensureNodeKeyStored(key); err != nil {
			e.slogger.Log(ctx, slog.LevelError,
				"could not update registration",
				"err", err,
			)
		}
		return e.NodeKey, false, nil
	}

	e.slogger.Log(ctx, slog.LevelInfo,
		"no node key found, starting enrollment",
	)
	span.AddEvent("starting_enrollment")

	enrollSecret, err := e.knapsack.ReadEnrollSecret()
	if err != nil {
		return "", true, fmt.Errorf("could not read enroll secret: %w", err)
	}

	identifier, err := e.getHostIdentifier()
	if err != nil {
		return "", true, fmt.Errorf("generating UUID: %w", err)
	}

	var enrollDetails types.EnrollmentDetails

	if err := backoff.WaitFor(func() error {
		details := e.knapsack.GetEnrollmentDetails()
		if details.OSVersion == "" || details.Hostname == "" {
			return fmt.Errorf("incomplete enrollment details (missing hostname or os version): %+v", details)
		}
		enrollDetails = details
		span.AddEvent("got_complete_enrollment_details")
		return nil
	}, 60*time.Second, 5*time.Second); err != nil {
		e.slogger.Log(ctx, slog.LevelWarn,
			"could not fetch enrollment details before timeout",
			"err", err,
		)
		span.AddEvent("enrollment_details_timeout")
		// Get final details state even if incomplete, ie: the osquery details failed but we can still enroll using the Runtime details.
		enrollDetails = e.knapsack.GetEnrollmentDetails()
	}

	// If no cached node key, enroll for new node key
	// note that we set invalid two ways. Via the return, _or_ via isNodeInvaliderr
	keyString, invalid, err := e.serviceClient.RequestEnrollment(ctx, enrollSecret, identifier, enrollDetails)

	switch {
	case errors.Is(err, service.ErrDeviceDisabled{}):
		e.slogger.Log(ctx, slog.LevelInfo,
			"received device disabled error during enrollment, uninstalling",
		)
		uninstall.Uninstall(ctx, e.knapsack, true)
		// the uninstall call above will cause launcher to uninstall and exit
		// so we are returning the err here just incase something somehow
		// goes wrong with the uninstall
		return "", true, fmt.Errorf("device disabled, should have uninstalled: %w", err)

	case isNodeInvalidErr(err):
		invalid = true

	case err != nil:
		return "", true, fmt.Errorf("transport error getting queries: %w", err)

	default: // pass through no error
	}

	if invalid {
		if err == nil {
			err = errors.New("no further error")
		}
		err = fmt.Errorf("enrollment invalid: %w", err)
		observability.SetError(span, err)
		return "", true, err
	}

	// Save newly acquired node key if successful -- adding the registration
	// will do this.
	e.NodeKey = keyString
	if err := e.addRegistration(keyString, enrollSecret); err != nil {
		e.slogger.Log(ctx, slog.LevelError,
			"could not add new registration to store",
			"err", err,
		)
	}

	e.slogger.Log(ctx, slog.LevelInfo,
		"completed enrollment",
	)
	span.AddEvent("completed_enrollment")

	return e.NodeKey, false, nil
}

func (e *Extension) enrolled() bool {
	// grab a reference to the existing nodekey to prevent data races with any re-enrollments
	e.enrollMutex.Lock()
	nodeKey := e.NodeKey
	e.enrollMutex.Unlock()

	return nodeKey != ""
}

// RequireReenroll clears the existing node key information, ensuring that the
// next call to Enroll will cause the enrollment process to take place.
func (e *Extension) RequireReenroll(ctx context.Context) {
	e.enrollMutex.Lock()
	defer e.enrollMutex.Unlock()
	// Clear the node key such that reenrollment is required.
	e.NodeKey = ""
	e.knapsack.ConfigStore().Delete(storage.KeyByIdentifier([]byte(nodeKeyKey), storage.IdentifierTypeRegistration, []byte(e.registrationId)))
	e.knapsack.RegistrationStore().Delete([]byte(e.registrationId))
}

// GenerateConfigs will request the osquery configuration from the server. If
// retrieving the configuration from the server fails, the locally stored
// configuration will be returned. If that fails, this method will return an
// error.
func (e *Extension) GenerateConfigs(ctx context.Context) (map[string]string, error) {
	config, err := e.generateConfigsWithReenroll(ctx, true)
	if err != nil {
		e.slogger.Log(ctx, slog.LevelDebug,
			"generating configs with reenroll failed",
			"err", err,
		)
		// Try to use cached config
		var confBytes []byte
		confBytes, _ = e.knapsack.ConfigStore().Get(storage.KeyByIdentifier([]byte(configKey), storage.IdentifierTypeRegistration, []byte(e.registrationId)))

		if len(confBytes) == 0 {
			if !e.enrolled() {
				// Not enrolled yet -- return an empty config
				return map[string]string{"config": "{}"}, nil
			}
			return nil, fmt.Errorf("loading config failed, no cached config: %w", err)
		}
		config = string(confBytes)
	} else {
		// Store good config in both the knapsack and our settings store
		if err := e.knapsack.ConfigStore().Set(storage.KeyByIdentifier([]byte(configKey), storage.IdentifierTypeRegistration, []byte(e.registrationId)), []byte(config)); err != nil {
			e.slogger.Log(ctx, slog.LevelError,
				"writing config to config store",
				"err", err,
			)
		}
		if err := e.settingsWriter.WriteSettings(); err != nil {
			e.slogger.Log(ctx, slog.LevelError,
				"writing config to startup settings",
				"err", err,
			)
		}
	}

	return map[string]string{"config": config}, nil
}

// TODO: https://github.com/kolide/launcher/issues/366
var reenrollmentInvalidErr = errors.New("enrollment invalid, reenrollment invalid")

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) generateConfigsWithReenroll(ctx context.Context, reenroll bool) (string, error) {
	// grab a reference to the existing nodekey to prevent data races with any re-enrollments
	e.enrollMutex.Lock()
	nodeKey := e.NodeKey
	e.enrollMutex.Unlock()

	config, invalid, err := e.serviceClient.RequestConfig(ctx, nodeKey)
	switch {
	case errors.Is(err, service.ErrDeviceDisabled{}):
		e.slogger.Log(ctx, slog.LevelInfo,
			"received device disabled error during config request, uninstalling",
		)
		uninstall.Uninstall(ctx, e.knapsack, true)
		// the uninstall call above will cause launcher to uninstall and exit
		// so we are returning the err here just incase something somehow
		// goes wrong with the uninstall
		return "", fmt.Errorf("device disabled, should have uninstalled: %w", err)

	case isNodeInvalidErr(err):
		invalid = true

	case err != nil:
		return "", fmt.Errorf("transport error getting queries: %w", err)

	default: // pass through no error
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
	configOptsToSet := startupOsqueryConfigOptions
	osqHistory := e.knapsack.OsqueryHistory()
	if osqHistory != nil {
		if uptimeMins, err := osqHistory.LatestInstanceUptimeMinutes(e.registrationId); err == nil && uptimeMins >= 10 {
			// Only log the state change once -- RequestConfig happens every 5 mins
			if uptimeMins <= 15 {
				e.slogger.Log(ctx, slog.LevelDebug,
					"osquery has been up for more than 10 minutes, switching from startup settings to post-startup settings",
					"uptime_mins", uptimeMins,
				)
			}
			configOptsToSet = postStartupOsqueryConfigOptions
		}
	}

	config = e.setOsqueryOptions(config, configOptsToSet)

	// If this feature flag is set, we want to use cached data for scheduled queries that might otherwise
	// time out. We achieve this by updating the config to point to the tables holding the cached data.
	if e.knapsack.UseCachedDataForScheduledQueries() {
		// The kolide_windows_updates table is currently the only table we use this feature for.
		config = strings.ReplaceAll(config, "kolide_windows_updates", "kolide_windows_updates_cached")
	}

	return config, nil
}

// setOsqueryOptions modifies the given config to add the given options in `optsToSet`.
// The values in `optsToSet` will override any existing and conflicting option values
// within `config`.
func (e *Extension) setOsqueryOptions(config string, optsToSet map[string]any) string {
	var cfg map[string]any

	if config != "" {
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			e.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not unmarshal config, cannot set verbose",
				"err", err,
			)
			return config
		}
	} else {
		cfg = make(map[string]any)
	}

	var opts map[string]any
	if cfgOpts, ok := cfg["options"]; ok {
		opts, ok = cfgOpts.(map[string]any)
		if !ok {
			e.slogger.Log(context.TODO(), slog.LevelWarn,
				"config options are malformed, cannot set verbose",
			)
			return config
		}
	} else {
		opts = make(map[string]any)
	}

	for k, v := range optsToSet {
		opts[k] = v
	}

	cfg["options"] = opts

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		e.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not marshal config, cannot set verbose",
			"err", err,
		)
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
	case logger.LogTypeHealth, logger.LogTypeInit:
		return nil, fmt.Errorf("storing log type %v is unsupported", typ)
	default:
		return nil, fmt.Errorf("unknown log type: %v", typ)
	}
}

// writeAndPurgeLogs flushes the log buffers, writing up to
// Opts.MaxBytesPerBatch bytes in one run. If the logs write successfully, they
// will be deleted from the buffer. After writing (whether success or failure),
// logs over the maximum count will be purged to avoid unbounded growth of the
// buffers.
func (e *Extension) writeAndPurgeLogs() {
	for _, typ := range []logger.LogType{logger.LogTypeStatus, logger.LogTypeString} {
		originalBatchState := e.logPublicationState.CurrentValues()
		// Write logs
		err := e.writeBufferedLogsForType(typ)
		if err != nil {
			e.slogger.Log(context.TODO(), slog.LevelInfo,
				"sending logs",
				"type", typ.String(),
				"attempted_publication_state", originalBatchState,
				"err", err,
			)
		}

		// Purge overflow
		err = e.purgeBufferedLogsForType(typ)
		if err != nil {
			e.slogger.Log(context.TODO(), slog.LevelInfo,
				"purging logs",
				"type", typ.String(),
				"err", err,
			)
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
	var logIDs [][]byte
	bufferFilled := false
	totalBytes := 0
	err = store.ForEach(func(k, v []byte) error {
		// A somewhat cumbersome if block...
		//
		// 1. If the log is too big, skip it and mark for deletion.
		// 2. If the buffer would be too big with the log, break for
		// 3. Else append it
		//
		// Note that (1) must come first, otherwise (2) will always trigger.
		if e.logPublicationState.ExceedsCurrentBatchThreshold(len(v)) {
			// Discard logs that are too big
			logheadSize := minInt(len(v), 100)
			e.slogger.Log(context.TODO(), slog.LevelInfo,
				"dropped log",
				"log_id", k,
				"size", len(v),
				"limit", e.Opts.MaxBytesPerBatch,
				"loghead", string(v)[0:logheadSize],
			)
		} else if e.logPublicationState.ExceedsCurrentBatchThreshold(totalBytes + len(v)) {
			// Buffer is filled. Break the loop and come back later.
			return iterationTerminatedError{}
		} else {
			logs = append(logs, string(v))
			totalBytes += len(v)
		}

		// Note the logID for deletion. We do this by
		// making a copy of k. It is retained in
		// logIDs after the transaction is closed,
		// when the goroutine ticks it zeroes out some
		// of the IDs to delete below, causing logs to
		// remain in the buffer and be sent again to
		// the server.
		logID := make([]byte, len(k))
		copy(logID, k)
		logIDs = append(logIDs, logID)
		return nil
	})

	if err != nil && errors.Is(err, iterationTerminatedError{}) {
		bufferFilled = true
	} else if err != nil {
		return fmt.Errorf("reading buffered logs: %w", err)
	}

	if len(logs) == 0 {
		// Nothing to send
		return nil
	}

	// inform the publication state tracking whether this batch should be used to
	// determine the appropriate limit
	e.logPublicationState.BeginBatch(time.Now(), bufferFilled)
	publicationCtx := context.WithValue(context.Background(),
		service.PublicationCtxKey,
		e.logPublicationState.CurrentValues(),
	)
	err = e.writeLogsWithReenroll(publicationCtx, typ, logs, true)
	if err != nil {
		return fmt.Errorf("writing logs: %w", err)
	}

	// Delete logs that were successfully sent
	err = store.Delete(logIDs...)

	if err != nil {
		return fmt.Errorf("deleting sent logs: %w", err)
	}

	return nil
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) writeLogsWithReenroll(ctx context.Context, typ logger.LogType, logs []string, reenroll bool) error {
	// grab a reference to the existing nodekey to prevent data races with any re-enrollments
	e.enrollMutex.Lock()
	nodeKey := e.NodeKey
	e.enrollMutex.Unlock()

	_, _, invalid, err := e.serviceClient.PublishLogs(ctx, nodeKey, typ, logs)

	if errors.Is(err, service.ErrDeviceDisabled{}) {
		e.slogger.Log(ctx, slog.LevelInfo,
			"received device disabled error during log publish, uninstalling",
		)
		uninstall.Uninstall(ctx, e.knapsack, true)
		// the uninstall call above will cause launcher to uninstall and exit
		// so we are returning the err here just incase something somehow
		// goes wrong with the uninstall
		return fmt.Errorf("device disabled, should have uninstalled: %w", err)
	}

	invalid = invalid || isNodeInvalidErr(err)
	if !invalid && err == nil {
		// publication was successful- update logPublicationState and move on
		e.logPublicationState.EndBatch(logs, true)
		return nil
	}

	if err != nil {
		// logPublicationState will determine whether this failure should impact
		// the batch size limit based on the elapsed time
		e.logPublicationState.EndBatch(logs, false)
		return fmt.Errorf("transport error sending logs: %w", err)
	}

	if !reenroll {
		return errors.New("enrollment invalid, reenroll disabled")
	}

	e.RequireReenroll(ctx)
	_, invalid, err = e.Enroll(ctx)
	if err != nil {
		return fmt.Errorf("enrollment invalid, reenrollment errored: %w", err)
	}
	if invalid {
		return errors.New("enrollment invalid, reenrollment invalid")
	}

	// Don't attempt reenroll after first attempt
	return e.writeLogsWithReenroll(ctx, typ, logs, false)
}

// purgeBufferedLogsForType flushes the log buffers for the provided type,
// ensuring that at most Opts.MaxBufferedLogs logs remain.
func (e *Extension) purgeBufferedLogsForType(typ logger.LogType) error {
	store, err := storeForLogType(e.knapsack, typ)
	if err != nil {
		return err
	}

	totalCount, err := store.Count()
	if err != nil {
		return err
	}

	deleteCount := totalCount - e.Opts.MaxBufferedLogs
	if deleteCount <= 0 { // Limit not exceeded
		return nil
	}

	logIdsCollectedCount := 0
	logIDsForDeletion := make([][]byte, deleteCount)
	if err = store.ForEach(func(k, v []byte) error {
		if logIdsCollectedCount >= deleteCount {
			return iterationTerminatedError{}
		}

		logID := make([]byte, len(k))
		copy(logID, k)
		logIDsForDeletion = append(logIDsForDeletion, logID)
		logIdsCollectedCount++
		return nil
	}); err != nil && !errors.Is(err, iterationTerminatedError{}) {
		return fmt.Errorf("collecting overflowed log keys for deletion: %w", err)
	}

	return store.Delete(logIDsForDeletion...)
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
		e.slogger.Log(ctx, slog.LevelInfo,
			"received unknown log type",
			"log_type", typ.String(),
		)
		return fmt.Errorf("unknown log type: %w", err)
	}

	// Buffer the log for sending later in a batch
	// note that AppendValues guarantees these logs are inserted with
	// sequential keys for ordered retrieval later
	return store.AppendValues([]byte(logText))
}

// GetQueries will request the distributed queries to execute from the server.
func (e *Extension) GetQueries(ctx context.Context) (*distributed.GetQueriesResult, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// Check to see whether we should forward this request --
	// 1. Check to see if we currently want to forward all distributed query requests to the cloud.
	// 2. Check to see if we forwarded a request to the cloud within the last minute.
	now := time.Now().Unix()
	if now >= e.forwardAllDistributedUntil.Load() && now < e.lastRequestQueriesTimestamp.Load()+e.distributedForwardingInterval.Load() {
		// Return an empty result to osquery.
		return &distributed.GetQueriesResult{}, nil
	}

	// We haven't requested queries from the cloud within the last minute --
	// forward this request.
	e.lastRequestQueriesTimestamp.Store(now)

	queries, err := e.getQueriesWithReenroll(ctx, true)
	if err != nil {
		return nil, err
	}

	// Check if the cloud wants us to accelerate distributed requests by forwarding
	// all requests to the cloud for the next `queries.AccelerateSeconds` seconds.
	if queries.AccelerateSeconds > 0 {
		// Store the timestamp when the acceleration ends
		e.forwardAllDistributedUntil.Store(time.Now().Unix() + int64(queries.AccelerateSeconds))
	}

	return queries, nil
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) getQueriesWithReenroll(ctx context.Context, reenroll bool) (*distributed.GetQueriesResult, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// grab a reference to the existing nodekey to prevent data races with any re-enrollments
	e.enrollMutex.Lock()
	nodeKey := e.NodeKey
	e.enrollMutex.Unlock()

	// Note that we set invalid two ways -- in the return, and via isNodeinvaliderr
	queries, invalid, err := e.serviceClient.RequestQueries(ctx, nodeKey)

	switch {
	case errors.Is(err, service.ErrDeviceDisabled{}):
		e.slogger.Log(ctx, slog.LevelInfo,
			"received device disabled error during queries request, uninstalling",
		)
		uninstall.Uninstall(ctx, e.knapsack, true)
		// the uninstall call above will cause launcher to uninstall and exit
		// so we are returning the err here just incase something somehow
		// goes wrong with the uninstall
		return nil, fmt.Errorf("device disabled, should have uninstalled: %w", err)

	case isNodeInvalidErr(err):
		invalid = true

	case err != nil:
		return nil, fmt.Errorf("transport error getting queries: %w", err)

	default: // pass through no error
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
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	return e.writeResultsWithReenroll(ctx, results, true)
}

// Helper to allow for a single attempt at re-enrollment
func (e *Extension) writeResultsWithReenroll(ctx context.Context, results []distributed.Result, reenroll bool) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// grab a reference to the existing nodekey to prevent data races with any re-enrollments
	e.enrollMutex.Lock()
	nodeKey := e.NodeKey
	e.enrollMutex.Unlock()

	_, _, invalid, err := e.serviceClient.PublishResults(ctx, nodeKey, results)
	switch {
	case errors.Is(err, service.ErrDeviceDisabled{}):
		e.slogger.Log(ctx, slog.LevelInfo,
			"received device disabled error during results publish, uninstalling",
		)
		uninstall.Uninstall(ctx, e.knapsack, true)
		// the uninstall call above will cause launcher to uninstall and exit
		// so we are returning the err here just incase something somehow
		// goes wrong with the uninstall
		return fmt.Errorf("device disabled, should have uninstalled: %w", err)

	case isNodeInvalidErr(err):
		invalid = true

	case err != nil:
		return fmt.Errorf("transport error getting queries: %w", err)

	default: // pass through no error
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

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

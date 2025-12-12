package knapsack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"go.etcd.io/bbolt"
)

// Package-level runID variable
var runID string

// type alias Flags, so that we can embed it inside knapsack, as `flags` and not `Flags`
type flags types.Flags

// nodeKeyKey is the key that we store the node key under in the config store.
var nodeKeyKey = []byte("nodeKey")

// enrollmentDetailsKey is the key that we store the enrollment details under in the enrollment details store.
var enrollmentDetailsKey = []byte("current_enrollment_details")

// Knapsack is an inventory of data and useful services which are used throughout
// launcher code and are typically valid for the lifetime of the launcher application instance.
type knapsack struct {
	stores map[storage.Store]types.KVStore
	// Embed flags so we get all the flag interfaces
	flags

	// BboltDB is the underlying bbolt database.
	// Ideally, we can eventually remove this. This is only here because some parts of the codebase
	// like the osquery extension have a direct dependency on bbolt and need this reference.
	// If we are able to abstract bbolt out completely in these areas, we should be able to
	// remove this field and prevent "leaking" bbolt into places it doesn't need to.
	db *bbolt.DB

	slogger, systemSlogger *multislogger.MultiSlogger

	querier types.InstanceQuerier

	osqHistory types.OsqueryHistorian

	desktopRunner types.DesktopRunner

	osqueryPublisher types.OsqueryPublisher
	// This struct is a work in progress, and will be iteratively added to as needs arise.
}

func New(stores map[storage.Store]types.KVStore, flags types.Flags, db *bbolt.DB, slogger, systemSlogger *multislogger.MultiSlogger) *knapsack {
	if slogger == nil {
		slogger = multislogger.New()
	}
	if systemSlogger == nil {
		systemSlogger = multislogger.New()
	}

	k := &knapsack{
		db:            db,
		flags:         flags,
		stores:        stores,
		slogger:       slogger,
		systemSlogger: systemSlogger,
	}

	return k
}

// GetRunID returns the current launcher run ID -- if it's not yet set, it will generate and set it
func (k *knapsack) GetRunID() string {
	if runID == "" {
		runID = ulid.New()
		k.slogger.Logger = k.slogger.Logger.With("run_id", runID)
		k.systemSlogger.Logger = k.systemSlogger.Logger.With("run_id", runID)
	}
	return runID
}

// Logging interface methods
func (k *knapsack) Slogger() *slog.Logger {
	return k.slogger.Logger
}

func (k *knapsack) SystemSlogger() *slog.Logger {
	return k.systemSlogger.Logger
}

func (k *knapsack) AddSlogHandler(handler ...slog.Handler) {
	k.slogger.AddHandler(handler...)
	k.systemSlogger.AddHandler(handler...)
}

// Osquery instance querier
func (k *knapsack) SetInstanceQuerier(q types.InstanceQuerier) {
	k.querier = q
}

// RegistrationTracker interface methods
func (k *knapsack) RegistrationIDs() []string {
	return []string{types.DefaultRegistrationID}
}

func (k *knapsack) Registrations() ([]types.Registration, error) {
	registrations := make([]types.Registration, 0)
	registrationStore := k.getKVStore(storage.RegistrationStore)
	if registrationStore == nil {
		return nil, errors.New("no registration store")
	}
	if err := registrationStore.ForEach(func(k []byte, v []byte) error {
		var r types.Registration
		if err := json.Unmarshal(v, &r); err != nil {
			return fmt.Errorf("unmarshalling registration %s: %w", string(k), err)
		}
		registrations = append(registrations, r)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("fetching registrations from store: %w", err)
	}
	return registrations, nil
}

// SaveRegistration creates a new registration using the given information and stores it
// in our registration store; it also stores the node key separately in the config store.
// It is permissible for the enrollment secret to be empty, in the case of a secretless enrollment.
// It is permissible for the munemo to be empty, if the enrollment secret is not -- we can
// extract the munemo from the enrollment secret.
func (k *knapsack) SaveRegistration(registrationId, munemo, nodeKey, enrollmentSecret string) error {
	// First, get the stores we'll need
	nodeKeyStore := k.getKVStore(storage.ConfigStore)
	if nodeKeyStore == nil {
		return errors.New("no config store")
	}
	registrationStore := k.getKVStore(storage.RegistrationStore)
	if registrationStore == nil {
		return errors.New("no registration store")
	}

	// Ensure we have the minimum information required to save a registration
	if munemo == "" && enrollmentSecret == "" {
		return errors.New("munemo and enrollment secret cannot both be empty")
	}
	if nodeKey == "" {
		return errors.New("node key cannot be empty")
	}

	// If we don't have a munemo, but do have an enrollment secret, attempt to extract the munemo
	// from the enrollment secret.
	if munemo == "" && enrollmentSecret != "" {
		// Extract the munemo from the enroll secret.
		// We do not have the key, and thus cannot verify -- so we use ParseUnverified.
		token, _, err := new(jwt.Parser).ParseUnverified(enrollmentSecret, jwt.MapClaims{})
		if err != nil {
			return fmt.Errorf("parsing enrollment secret: %w", err)
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return errors.New("no claims in enrollment secret")
		}
		munemoClaim, munemoFound := claims["organization"]
		if !munemoFound {
			return errors.New("no claim for organization in enrollment secret, cannot get munemo")
		}
		munemo = fmt.Sprintf("%s", munemoClaim)
	}

	// Prepare the new registration for storage
	r := types.Registration{
		RegistrationID:   registrationId,
		Munemo:           munemo,
		NodeKey:          nodeKey,
		EnrollmentSecret: enrollmentSecret,
	}
	rawRegistration, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshalling registration: %w", err)
	}

	// Now, store our data
	nodeKeyKeyForRegistration := storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeRegistration, []byte(registrationId))
	if err := nodeKeyStore.Set(nodeKeyKeyForRegistration, []byte(nodeKey)); err != nil {
		return fmt.Errorf("setting node key in store: %w", err)
	}
	if err := registrationStore.Set([]byte(registrationId), rawRegistration); err != nil {
		return fmt.Errorf("adding registration to store: %w", err)
	}

	k.Slogger().Log(context.Background(), slog.LevelInfo,
		"successfully saved registration",
		"node_key_key", string(nodeKeyKeyForRegistration),
		"registration_id", registrationId,
		"munemo", munemo,
	)

	return nil
}

// EnsureRegistrationStored creates a registration under the given registration ID,
// if it does not already exist. Since registrations are a newer concept, older launcher
// installations may not have them saved yet.
func (k *knapsack) EnsureRegistrationStored(registrationId string) error {
	// First, get the stores we'll need
	nodeKeyStore := k.getKVStore(storage.ConfigStore)
	if nodeKeyStore == nil {
		return errors.New("no config store, cannot ensure registration is stored")
	}
	registrationStore := k.getKVStore(storage.RegistrationStore)
	if registrationStore == nil {
		return errors.New("no registration store, cannot ensure registration is stored")
	}

	// Get the existing node key from the config store. If we can't get this, then
	// there's nothing more we can do here -- we won't save a registration without
	// a node key.
	nodeKeyKeyForRegistration := storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeRegistration, []byte(registrationId))
	keyRaw, err := nodeKeyStore.Get(nodeKeyKeyForRegistration)
	if err != nil {
		return fmt.Errorf("retrieving node key: %w", err)
	}
	nodeKey := string(keyRaw)
	if nodeKey == "" {
		return errors.New("no node key stored, cannot store registration (probably a new install)")
	}

	// Check to see if we have an existing registration first
	existingRegistrationRaw, err := registrationStore.Get([]byte(registrationId))
	if err != nil {
		return fmt.Errorf("getting existing registration: %w", err)
	}

	// No registration exists yet -- add it if we can
	if existingRegistrationRaw == nil {
		// Grab the enroll secret, since we need that to create a new registration.
		enrollSecret, err := k.ReadEnrollSecret()
		if err != nil {
			// The enroll secret doesn't exist -- likely, this is a bad manual installation.
			return fmt.Errorf("reading enrollment secret to create new registration: %w", err)
		}
		// SaveRegistration will extract the munemo from the enrollment secret for us.
		if err := k.SaveRegistration(registrationId, "", nodeKey, enrollSecret); err != nil {
			return fmt.Errorf("saving new registration: %w", err)
		}
		return nil
	}

	// We have an existing registration, which may or may not have the node key set on it yet --
	// unmarshal it so we can update it appropriately with the node key.
	var existingRegistration types.Registration
	if err := json.Unmarshal(existingRegistrationRaw, &existingRegistration); err != nil {
		return fmt.Errorf("unmarshalling existing registration: %w", err)
	}

	// Registration already exists, and the node key is correct -- nothing to do here!
	if nodeKey == existingRegistration.NodeKey {
		return nil
	}

	// Make update
	existingRegistration.NodeKey = nodeKey
	updatedRegistrationRaw, err := json.Marshal(existingRegistration)
	if err != nil {
		return fmt.Errorf("marshalling updated registration: %w", err)
	}
	if err := registrationStore.Set([]byte(registrationId), updatedRegistrationRaw); err != nil {
		return fmt.Errorf("updating registration in store: %w", err)
	}

	k.Slogger().Log(context.TODO(), slog.LevelInfo,
		"successfully updated registration's node key",
		"munemo", existingRegistration.Munemo,
		"registration_id", registrationId,
	)
	return nil
}

func (k *knapsack) NodeKey(registrationId string) (string, error) {
	// First, get the store we'll need. For now, we continue to pull the node key from
	// the config store, but in the future, we will be able to pull it from the registrations
	// store instead.
	nodeKeyStore := k.getKVStore(storage.ConfigStore)
	if nodeKeyStore == nil {
		return "", errors.New("no config store")
	}

	// Retrieve the key from the store
	key, err := nodeKeyStore.Get(storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeRegistration, []byte(registrationId)))
	if err != nil {
		return "", fmt.Errorf("error getting node key: %w", err)
	}

	if key != nil {
		return string(key), nil
	}

	return "", nil
}

func (k *knapsack) DeleteRegistration(registrationId string) error {
	// First, get the stores we'll need
	nodeKeyStore := k.getKVStore(storage.ConfigStore)
	if nodeKeyStore == nil {
		return errors.New("no config store")
	}
	registrationStore := k.getKVStore(storage.RegistrationStore)
	if registrationStore == nil {
		return errors.New("no registration store")
	}

	if err := nodeKeyStore.Delete(storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeRegistration, []byte(registrationId))); err != nil {
		return fmt.Errorf("deleting node key for %s: %w", registrationId, err)
	}
	if err := registrationStore.Delete([]byte(registrationId)); err != nil {
		return fmt.Errorf("deleting registration for %s: %w", registrationId, err)
	}

	k.Slogger().Log(context.Background(), slog.LevelInfo,
		"deleted registration",
		"registration_id", registrationId,
	)

	return nil
}

// InstanceStatuses returns the current status of each osquery instance.
// It performs a healthcheck against each existing instance.
func (k *knapsack) InstanceStatuses() map[string]types.InstanceStatus {
	if k.querier == nil {
		return nil
	}
	return k.querier.InstanceStatuses()
}

// BboltDB interface methods
func (k *knapsack) BboltDB() *bbolt.DB {
	return k.db
}

// Stores interface methods
func (k *knapsack) Stores() map[storage.Store]types.KVStore {
	return k.stores
}

func (k *knapsack) AgentFlagsStore() types.KVStore {
	return k.getKVStore(storage.AgentFlagsStore)
}

func (k *knapsack) KatcConfigStore() types.KVStore {
	return k.getKVStore(storage.KatcConfigStore)
}

func (k *knapsack) ConfigStore() types.KVStore {
	return k.getKVStore(storage.ConfigStore)
}

func (k *knapsack) ControlStore() types.KVStore {
	return k.getKVStore(storage.ControlStore)
}

func (k *knapsack) PersistentHostDataStore() types.KVStore {
	return k.getKVStore(storage.PersistentHostDataStore)
}

func (k *knapsack) InitialResultsStore() types.KVStore {
	return k.getKVStore(storage.InitialResultsStore)
}

func (k *knapsack) ResultLogsStore() types.KVStore {
	return k.getKVStore(storage.ResultLogsStore)
}

func (k *knapsack) OsqueryHistoryInstanceStore() types.KVStore {
	return k.getKVStore(storage.OsqueryHistoryInstanceStore)
}

func (k *knapsack) SentNotificationsStore() types.KVStore {
	return k.getKVStore(storage.SentNotificationsStore)
}

func (k *knapsack) ControlServerActionsStore() types.KVStore {
	return k.getKVStore(storage.ControlServerActionsStore)
}

func (k *knapsack) StatusLogsStore() types.KVStore {
	return k.getKVStore(storage.StatusLogsStore)
}

func (k *knapsack) ServerProvidedDataStore() types.KVStore {
	return k.getKVStore(storage.ServerProvidedDataStore)
}

func (k *knapsack) TokenStore() types.KVStore {
	return k.getKVStore(storage.TokenStore)
}

func (k *knapsack) LauncherHistoryStore() types.KVStore {
	return k.getKVStore(storage.LauncherHistoryStore)
}

func (k *knapsack) Dt4aInfoStore() types.KVStore {
	return k.getKVStore(storage.Dt4aInfoStore)
}

func (k *knapsack) WindowsUpdatesCacheStore() types.KVStore {
	return k.getKVStore(storage.WindowsUpdatesCacheStore)
}

func (k *knapsack) RegistrationStore() types.KVStore {
	return k.getKVStore(storage.RegistrationStore)
}

func (k *knapsack) EnrollmentDetailsStore() types.KVStore {
	return k.getKVStore(storage.EnrollmentDetailsStore)
}

func (k *knapsack) SetLauncherWatchdogDisabled(disabled bool) error {
	return k.flags.SetLauncherWatchdogDisabled(disabled)
}
func (k *knapsack) LauncherWatchdogDisabled() bool {
	return k.flags.LauncherWatchdogDisabled()
}

func (k *knapsack) getKVStore(storeType storage.Store) types.KVStore {
	if k == nil {
		return nil
	}

	// Ignoring ok value, this should only fail if an invalid storeType is provided
	store := k.stores[storeType]
	return store
}

func (k *knapsack) LatestOsquerydPath(ctx context.Context) string {
	latestBin, err := tuf.CheckOutLatest(ctx, "osqueryd", k.RootDirectory(), k.UpdateDirectory(), k.PinnedOsquerydVersion(), k.UpdateChannel(), k.Slogger())
	if err != nil {
		return k.OsquerydPath()
	}
	k.SetCurrentRunningOsqueryVersion(latestBin.Version)
	return latestBin.Path
}

func (k *knapsack) ReadEnrollSecret() (string, error) {
	if k.EnrollSecret() != "" {
		return k.EnrollSecret(), nil
	}

	if k.EnrollSecretPath() != "" {
		content, err := os.ReadFile(k.EnrollSecretPath())
		if err != nil {
			return "", fmt.Errorf("could not read enroll secret path %s: %w", k.EnrollSecretPath(), err)
		}
		return string(bytes.TrimSpace(content)), nil
	}

	return "", errors.New("enroll secret not set")
}

func (k *knapsack) CurrentEnrollmentStatus() (types.EnrollmentStatus, error) {
	// Check to see if we have a node key for the default registration.
	key, err := k.NodeKey(types.DefaultRegistrationID)
	if err != nil {
		return types.Unknown, fmt.Errorf("getting node key from store: %w", err)
	}

	if len(key) > 0 {
		return types.Enrolled, nil
	}

	// We are unenrolled. Check to see if we're a secretless installation.
	enrollSecret, err := k.ReadEnrollSecret()
	if err != nil || enrollSecret == "" {
		return types.NoEnrollmentKey, nil
	}

	// Not secretless, merely unenrolled
	return types.Unenrolled, nil
}

// SetEnrollmentDetails updates the enrollment details, merging with existing details
func (k *knapsack) SetEnrollmentDetails(newDetails types.EnrollmentDetails) {
	store := k.EnrollmentDetailsStore()
	if store == nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"enrollment details store not available",
		)
		return
	}

	// Get existing details
	oldDetails := k.GetEnrollmentDetails()

	// Merge the new details with existing ones
	mergedDetails := mergeEnrollmentDetails(oldDetails, newDetails)

	// Marshal to JSON
	data, err := json.Marshal(mergedDetails)
	if err != nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"failed to marshal enrollment details",
			"err", err,
		)
		return
	}

	// Save to store
	if err := store.Set(enrollmentDetailsKey, data); err != nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"failed to save enrollment details to store",
			"err", err,
		)
		return
	}

	// Log old and merged details
	k.Slogger().Log(context.Background(), slog.LevelDebug,
		"updated enrollment details in store",
		"old_details", fmt.Sprintf("%+v", oldDetails),
		"new_details", fmt.Sprintf("%+v", mergedDetails),
	)
}

// mergeEnrollmentDetails combines old and new details, only updating non-empty fields
func mergeEnrollmentDetails(oldDetails, newDetails types.EnrollmentDetails) types.EnrollmentDetails {
	// Start with existing details
	mergedDetails := oldDetails

	// Only update fields that are not empty
	if newDetails.OSPlatform != "" {
		mergedDetails.OSPlatform = newDetails.OSPlatform
	}
	if newDetails.OSPlatformLike != "" {
		mergedDetails.OSPlatformLike = newDetails.OSPlatformLike
	}
	if newDetails.OSVersion != "" {
		mergedDetails.OSVersion = newDetails.OSVersion
	}
	if newDetails.OSBuildID != "" {
		mergedDetails.OSBuildID = newDetails.OSBuildID
	}
	if newDetails.Hostname != "" {
		mergedDetails.Hostname = newDetails.Hostname
	}
	if newDetails.HardwareSerial != "" {
		mergedDetails.HardwareSerial = newDetails.HardwareSerial
	}
	if newDetails.HardwareModel != "" {
		mergedDetails.HardwareModel = newDetails.HardwareModel
	}
	if newDetails.HardwareVendor != "" {
		mergedDetails.HardwareVendor = newDetails.HardwareVendor
	}
	if newDetails.HardwareUUID != "" {
		mergedDetails.HardwareUUID = newDetails.HardwareUUID
	}
	if newDetails.OsqueryVersion != "" {
		mergedDetails.OsqueryVersion = newDetails.OsqueryVersion
	}
	if newDetails.LauncherVersion != "" {
		mergedDetails.LauncherVersion = newDetails.LauncherVersion
	}
	if newDetails.GOOS != "" {
		mergedDetails.GOOS = newDetails.GOOS
	}
	if newDetails.GOARCH != "" {
		mergedDetails.GOARCH = newDetails.GOARCH
	}
	if newDetails.LauncherLocalKey != "" {
		mergedDetails.LauncherLocalKey = newDetails.LauncherLocalKey
	}
	if newDetails.LauncherHardwareKey != "" {
		mergedDetails.LauncherHardwareKey = newDetails.LauncherHardwareKey
	}
	if newDetails.LauncherHardwareKeySource != "" {
		mergedDetails.LauncherHardwareKeySource = newDetails.LauncherHardwareKeySource
	}
	if newDetails.OSName != "" {
		mergedDetails.OSName = newDetails.OSName
	}

	return mergedDetails
}

func (k *knapsack) GetEnrollmentDetails() types.EnrollmentDetails {
	store := k.EnrollmentDetailsStore()
	if store == nil {
		k.Slogger().Log(context.Background(), slog.LevelDebug,
			"enrollment details store not available",
		)
		return types.EnrollmentDetails{}
	}

	// Read from store
	data, err := store.Get(enrollmentDetailsKey)
	if err != nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"failed to get enrollment details from store",
			"err", err,
		)
		return types.EnrollmentDetails{}
	}

	if data == nil {
		return types.EnrollmentDetails{}
	}

	// Unmarshal
	var details types.EnrollmentDetails
	if err := json.Unmarshal(data, &details); err != nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"failed to unmarshal enrollment details",
			"err", err,
		)
		return types.EnrollmentDetails{}
	}

	// Refresh osquery version, covers the case where the osquery version is updated without launcher restart
	// Only attempt this if flags is available
	if k.flags != nil {
		osqueryVersion := k.CurrentRunningOsqueryVersion()
		details.OsqueryVersion = osqueryVersion
	}

	return details
}

func (k *knapsack) OsqueryHistory() types.OsqueryHistorian {
	return k.osqHistory
}

func (k *knapsack) SetOsqueryHistory(osqHistory types.OsqueryHistorian) {
	k.osqHistory = osqHistory
}

// OsqueryPublisher returns the osquery publisher client
func (k *knapsack) OsqueryPublisher() types.OsqueryPublisher {
	return k.osqueryPublisher
}

// SetOsqueryPublisher sets the osquery publisher client
func (k *knapsack) SetOsqueryPublisher(p types.OsqueryPublisher) {
	k.osqueryPublisher = p
}

// DesktopRunner interface methods
func (k *knapsack) RequestProfile(ctx context.Context, profileType string) ([]string, error) {
	if k.desktopRunner == nil {
		return nil, nil
	}
	return k.desktopRunner.RequestProfile(ctx, profileType)
}

func (k *knapsack) SetDesktopRunner(runner types.DesktopRunner) {
	k.desktopRunner = runner
}

func (k *knapsack) ServerReleaseTrackerDataStore() types.KVStore {
	return k.getKVStore(storage.ServerReleaseTrackerDataStore)
}

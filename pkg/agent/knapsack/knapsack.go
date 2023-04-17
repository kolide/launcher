package knapsack

import (
	"time"

	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"go.etcd.io/bbolt"
)

// Knapsack is an inventory of data and useful services which are used throughout
// launcher code and are typically valid for the lifetime of the launcher application instance.
type knapsack struct {
	stores map[storage.Store]types.KVStore
	flags  types.Flags

	// BboltDB is the underlying bbolt database.
	// Ideally, we can eventually remove this. This is only here because some parts of the codebase
	// like the osquery extension have a direct dependency on bbolt and need this reference.
	// If we are able to abstract bbolt out completely in these areas, we should be able to
	// remove this field and prevent "leaking" bbolt into places it doesn't need to.
	db *bbolt.DB

	// This struct is a work in progress, and will be iteratively added to as needs arise.
	// Some potential future additions include:
	// Querier
}

func New(stores map[storage.Store]types.KVStore, flags types.Flags, db *bbolt.DB) *knapsack {
	k := &knapsack{
		db:     db,
		flags:  flags,
		stores: stores,
	}

	return k
}

func (k *knapsack) BboltDB() *bbolt.DB {
	return k.db
}

func (k *knapsack) getKVStore(storeType storage.Store) types.KVStore {
	if k == nil {
		return nil
	}

	// Ignoring ok value, this should only fail if an invalid storeType is provided
	store, _ := k.stores[storeType]
	return store
}

func (k *knapsack) AgentFlagsStore() types.KVStore {
	return k.getKVStore(storage.AgentFlagsStore)
}

func (k *knapsack) AutoupdateErrorsStore() types.KVStore {
	return k.getKVStore(storage.AutoupdateErrorsStore)
}

func (k *knapsack) ConfigStore() types.KVStore {
	return k.getKVStore(storage.ConfigStore)
}

func (k *knapsack) ControlStore() types.KVStore {
	return k.getKVStore(storage.ControlStore)
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

func (k *knapsack) StatusLogsStore() types.KVStore {
	return k.getKVStore(storage.StatusLogsStore)
}

func (k *knapsack) ServerProvidedDataStore() types.KVStore {
	return k.getKVStore(storage.ServerProvidedDataStore)
}

func (k *knapsack) RegisterChangeObserver(observer types.FlagsChangeObserver, flagKeys ...keys.FlagKey) {
	k.flags.RegisterChangeObserver(observer, flagKeys...)
}

func (k *knapsack) SetDesktopEnabled(enabled bool) error {
	return k.flags.SetDesktopEnabled(enabled)
}
func (k *knapsack) DesktopEnabled() bool {
	return k.flags.DesktopEnabled()
}

func (k *knapsack) SetDebugServerData(debug bool) error {
	return k.flags.SetDebugServerData(debug)
}
func (k *knapsack) DebugServerData() bool {
	return k.flags.DebugServerData()
}

func (k *knapsack) SetForceControlSubsystems(force bool) error {
	return k.flags.SetForceControlSubsystems(force)
}
func (k *knapsack) ForceControlSubsystems() bool {
	return k.flags.ForceControlSubsystems()
}

func (k *knapsack) SetControlServerURL(url string) error {
	return k.flags.SetControlServerURL(url)
}
func (k *knapsack) ControlServerURL() string {
	return k.flags.ControlServerURL()
}

func (k *knapsack) SetControlRequestInterval(interval time.Duration) error {
	return k.flags.SetControlRequestInterval(interval)
}
func (k *knapsack) SetControlRequestIntervalOverride(interval, duration time.Duration) {
	k.flags.SetControlRequestIntervalOverride(interval, duration)
}
func (k *knapsack) ControlRequestInterval() time.Duration {
	return k.flags.ControlRequestInterval()
}

func (k *knapsack) SetDisableControlTLS(disabled bool) error {
	return k.flags.SetDisableControlTLS(disabled)
}
func (k *knapsack) DisableControlTLS() bool {
	return k.flags.DisableControlTLS()
}

func (k *knapsack) SetInsecureControlTLS(disabled bool) error {
	return k.flags.SetInsecureControlTLS(disabled)
}
func (k *knapsack) InsecureControlTLS() bool {
	return k.flags.InsecureControlTLS()
}

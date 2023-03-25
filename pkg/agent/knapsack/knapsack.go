package knapsack

import (
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"go.etcd.io/bbolt"
)

// Knapsack is an inventory of data and useful services which are used throughout
// launcher code and are typically valid for the lifetime of the launcher application instance.
type Knapsack struct {
	stores map[storage.Store]types.KVStore
	Flags  flags.Flags

	// BboltDB is the underlying bbolt database.
	// Ideally, we can eventually remove this. This is only here because some parts of the codebase
	// like the osquery extension have a direct dependency on bbolt and need this reference.
	// If we are able to abstract bbolt out completely in these areas, we should be able to
	// remove this field and prevent "leaking" bbolt into places it doesn't need to.
	BboltDB *bbolt.DB

	// This struct is a work in progress, and will be iteratively added to as needs arise.
	// Some potential future additions include:
	// Querier
}

func New(stores map[storage.Store]types.KVStore, f flags.Flags, db *bbolt.DB) *Knapsack {
	k := &Knapsack{
		BboltDB: db,
		Flags:   f,
		stores:  stores,
	}

	return k
}

func (k *Knapsack) getKVStore(storeType storage.Store) types.KVStore {
	if k == nil {
		return nil
	}

	// Ignoring ok value, this should only fail if an invalid storeType is provided
	store, _ := k.stores[storeType]
	return store
}

func (k *Knapsack) AgentFlagsStore() types.KVStore {
	return k.getKVStore(storage.AgentFlagsStore)
}

func (k *Knapsack) AutoupdateErrorsStore() types.KVStore {
	return k.getKVStore(storage.AutoupdateErrorsStore)
}

func (k *Knapsack) ConfigStore() types.KVStore {
	return k.getKVStore(storage.ConfigStore)
}

func (k *Knapsack) ControlStore() types.KVStore {
	return k.getKVStore(storage.ControlStore)
}

func (k *Knapsack) InitialResultsStore() types.KVStore {
	return k.getKVStore(storage.InitialResultsStore)
}

func (k *Knapsack) ResultLogsStore() types.KVStore {
	return k.getKVStore(storage.ResultLogsStore)
}

func (k *Knapsack) OsqueryHistoryInstanceStore() types.KVStore {
	return k.getKVStore(storage.OsqueryHistoryInstanceStore)
}

func (k *Knapsack) SentNotificationsStore() types.KVStore {
	return k.getKVStore(storage.SentNotificationsStore)
}

func (k *Knapsack) StatusLogsStore() types.KVStore {
	return k.getKVStore(storage.StatusLogsStore)
}

func (k *Knapsack) ServerProvidedDataStore() types.KVStore {
	return k.getKVStore(storage.ServerProvidedDataStore)
}

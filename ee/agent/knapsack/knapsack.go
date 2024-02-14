package knapsack

import (
	"context"

	"log/slog"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"go.etcd.io/bbolt"
)

// type alias Flags, so that we can embed it inside knapsack, as `flags` and not `Flags`
type flags types.Flags

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

	// This struct is a work in progress, and will be iteratively added to as needs arise.
	// Some potential future additions include:
	// Querier
}

func New(stores map[storage.Store]types.KVStore, flags types.Flags, db *bbolt.DB, slogger, systemSlogger *multislogger.MultiSlogger) *knapsack {
	k := &knapsack{
		db:            db,
		flags:         flags,
		stores:        stores,
		slogger:       slogger,
		systemSlogger: systemSlogger,
	}

	return k
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

func (k *knapsack) AutoupdateErrorsStore() types.KVStore {
	return k.getKVStore(storage.AutoupdateErrorsStore)
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

func (k *knapsack) getKVStore(storeType storage.Store) types.KVStore {
	if k == nil {
		return nil
	}

	// Ignoring ok value, this should only fail if an invalid storeType is provided
	store := k.stores[storeType]
	return store
}

func (k *knapsack) LatestOsquerydPath(ctx context.Context) string {
	latestBin, err := tuf.CheckOutLatest(ctx, "osqueryd", k.RootDirectory(), k.UpdateDirectory(), k.UpdateChannel(), log.NewNopLogger())
	if err != nil {
		return autoupdate.FindNewest(ctx, k.OsquerydPath())
	}

	return latestBin.Path
}

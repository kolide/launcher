package knapsack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"log/slog"

	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

// type alias Flags, so that we can embed it inside knapsack, as `flags` and not `Flags`
type flags types.Flags

// Knapsack is an inventory of data and useful services which are used throughout
// launcher code and are typically valid for the lifetime of the launcher application instance.
type knapsack struct {
	stores map[storage.Store]types.KVStore
	// Embed flags so we get all the flag interfaces
	flags
	storageStatTracker     types.StorageStatTracker
	slogger, systemSlogger *multislogger.MultiSlogger

	// This struct is a work in progress, and will be iteratively added to as needs arise.
	// Some potential future additions include:
	// Querier
}

func New(stores map[storage.Store]types.KVStore, flags types.Flags, sStatTracker types.StorageStatTracker, slogger, systemSlogger *multislogger.MultiSlogger) *knapsack {
	if slogger == nil {
		slogger = multislogger.New()
	}
	if systemSlogger == nil {
		systemSlogger = multislogger.New()
	}

	k := &knapsack{
		storageStatTracker: sStatTracker,
		flags:              flags,
		stores:             stores,
		slogger:            slogger,
		systemSlogger:      systemSlogger,
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

// storage stat tracking interface methods
func (k *knapsack) StorageStatTracker() types.StorageStatTracker {
	return k.storageStatTracker
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
	latestBin, err := tuf.CheckOutLatest(ctx, "osqueryd", k.RootDirectory(), k.UpdateDirectory(), k.PinnedOsquerydVersion(), k.UpdateChannel(), k.Slogger())
	if err != nil {
		return autoupdate.FindNewest(ctx, k.OsquerydPath())
	}

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
	enrollSecret, err := k.ReadEnrollSecret()
	if err != nil || enrollSecret == "" {
		return types.NoEnrollmentKey, nil
	}

	if k.ConfigStore() == nil {
		return types.Unknown, errors.New("no config store in knapsack")
	}

	key, err := k.ConfigStore().Get([]byte("nodeKey"))
	if err != nil {
		return types.Unknown, fmt.Errorf("getting node key from store: %w", err)
	}

	if len(key) == 0 {
		return types.Unenrolled, nil
	}

	return types.Enrolled, nil
}

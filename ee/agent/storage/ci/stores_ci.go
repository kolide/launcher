package storageci

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"go.etcd.io/bbolt"

	"github.com/kolide/launcher/ee/agent/storage"
	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
)

// MakeStores creates all the KVStores used by launcher
func MakeStores(t *testing.T, slogger *slog.Logger, db *bbolt.DB) (map[storage.Store]types.KVStore, error) {
	var storeNames = []storage.Store{
		storage.AgentFlagsStore,
		storage.KatcConfigStore,
		storage.ConfigStore,
		storage.ControlStore,
		storage.PersistentHostDataStore,
		storage.InitialResultsStore,
		storage.ResultLogsStore,
		storage.OsqueryHistoryInstanceStore,
		storage.SentNotificationsStore,
		storage.StatusLogsStore,
		storage.ServerProvidedDataStore,
		storage.TokenStore,
		storage.LauncherHistoryStore,
		storage.Dt4aInfoStore,
		storage.WindowsUpdatesCacheStore,
		storage.RegistrationStore,
	}

	if os.Getenv("CI") == "true" {
		return makeInMemoryStores(t, storeNames), nil
	}

	return makeBboltStores(t, slogger, db, storeNames)
}

func makeInMemoryStores(t *testing.T, storeNames []storage.Store) map[storage.Store]types.KVStore {
	stores := make(map[storage.Store]types.KVStore)

	for _, storeName := range storeNames {
		stores[storeName] = inmemory.NewStore()
	}

	return stores
}

func makeBboltStores(t *testing.T, slogger *slog.Logger, db *bbolt.DB, storeNames []storage.Store) (map[storage.Store]types.KVStore, error) {
	stores := make(map[storage.Store]types.KVStore)

	for _, storeName := range storeNames {
		store, err := agentbbolt.NewStore(context.TODO(), slogger, db, storeName.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create '%s' KVStore: %w", storeName, err)
		}

		stores[storeName] = store
	}

	return stores, nil
}

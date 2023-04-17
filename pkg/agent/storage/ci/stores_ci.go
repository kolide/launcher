package storageci

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"go.etcd.io/bbolt"

	"github.com/kolide/launcher/pkg/agent/storage"
	agentbbolt "github.com/kolide/launcher/pkg/agent/storage/bbolt"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/agent/types"
)

// MakeStores creates all the KVStores used by launcher
func MakeStores(t *testing.T, logger log.Logger, db *bbolt.DB) (map[storage.Store]types.KVStore, error) {
	var storeNames = []storage.Store{
		storage.AgentFlagsStore,
		storage.AutoupdateErrorsStore,
		storage.ConfigStore,
		storage.ControlStore,
		storage.InitialResultsStore,
		storage.ResultLogsStore,
		storage.OsqueryHistoryInstanceStore,
		storage.SentNotificationsStore,
		storage.StatusLogsStore,
		storage.ServerProvidedDataStore,
	}

	if os.Getenv("CI") == "true" {
		return makeInMemoryStores(t, logger, storeNames), nil
	}

	return makeBboltStores(t, logger, db, storeNames)
}

func makeInMemoryStores(t *testing.T, logger log.Logger, storeNames []storage.Store) map[storage.Store]types.KVStore {
	stores := make(map[storage.Store]types.KVStore)

	for _, storeName := range storeNames {
		stores[storeName] = inmemory.NewStore(logger)
	}

	return stores
}

func makeBboltStores(t *testing.T, logger log.Logger, db *bbolt.DB, storeNames []storage.Store) (map[storage.Store]types.KVStore, error) {
	stores := make(map[storage.Store]types.KVStore)

	for _, storeName := range storeNames {
		store, err := agentbbolt.NewStore(logger, db, storeName.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create '%s' KVStore: %w", storeName, err)
		}

		stores[storeName] = store
	}

	return stores, nil
}

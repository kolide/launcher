package agentbbolt

import (
	"fmt"

	"github.com/go-kit/kit/log"

	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"go.etcd.io/bbolt"
)

// MakeStores creates all the KVStores used by launcher
func MakeStores(logger log.Logger, db *bbolt.DB) (map[storage.Store]types.KVStore, error) {
	stores := make(map[storage.Store]types.KVStore)

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

	for _, storeName := range storeNames {
		store, err := NewStore(logger, db, storeName.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create '%s' KVStore: %w", storeName, err)
		}

		stores[storeName] = store
	}

	return stores, nil
}

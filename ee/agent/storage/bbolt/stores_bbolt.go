package agentbbolt

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
	"go.etcd.io/bbolt"
)

// MakeStores creates all the KVStores used by launcher
func MakeStores(ctx context.Context, slogger *slog.Logger, db *bbolt.DB) (map[storage.Store]types.KVStore, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	stores := make(map[storage.Store]types.KVStore)

	var storeNames = []storage.Store{
		storage.AgentFlagsStore,
		storage.KatcConfigStore,
		storage.AutoupdateErrorsStore,
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
		storage.ControlServerActionsStore,
	}

	for _, storeName := range storeNames {
		store, err := NewStore(slogger, db, storeName.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create '%s' KVStore: %w", storeName, err)
		}

		stores[storeName] = store
	}

	return stores, nil
}

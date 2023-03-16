package agentbbolt

import (
	"fmt"

	"github.com/go-kit/kit/log"

	"github.com/kolide/launcher/pkg/agent/types"
	"go.etcd.io/bbolt"
)

type bboltStorage struct {
	logger log.Logger
	db     *bbolt.DB
	stores map[types.Store]*bboltKeyValueStore
}

func NewStorage(logger log.Logger, db *bbolt.DB) (*bboltStorage, error) {
	s := &bboltStorage{
		logger: logger,
		db:     db,
		stores: make(map[types.Store]*bboltKeyValueStore),
	}

	var storeNames = []types.Store{
		types.AgentFlagsStore,
		types.ConfigStore,
		types.ControlStore,
		types.InitialResultsStore,
		types.ResultLogsStore,
		types.OsqueryHistoryInstance,
		types.SentNotificationsStore,
		types.StatusLogsStore,
		types.ServerProvidedDataStore,
	}

	for _, storeName := range storeNames {
		store, err := NewStore(logger, db, storeName.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create '%s' KVStore: %w", storeName, err)
		}

		s.stores[storeName] = store
	}

	return s, nil
}

func (s *bboltStorage) GetStore(storeType types.Store) types.KVStore {
	// Ignoring ok value, this should only fail if an invalid storeType is provided
	store, _ := s.stores[storeType]
	return store
}

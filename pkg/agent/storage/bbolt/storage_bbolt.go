package agentbbolt

import (
	"fmt"

	"github.com/go-kit/kit/log"

	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"go.etcd.io/bbolt"
)

type bboltStorage struct {
	logger log.Logger
	db     *bbolt.DB
	stores map[storage.Store]*bboltKeyValueStore
}

func NewStorage(logger log.Logger, db *bbolt.DB) (*bboltStorage, error) {
	s := &bboltStorage{
		logger: logger,
		db:     db,
		stores: make(map[storage.Store]*bboltKeyValueStore),
	}

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

		s.stores[storeName] = store
	}

	return s, nil
}

func (s *bboltStorage) getKVStore(storeType storage.Store) types.KVStore {
	if s == nil {
		return nil
	}

	// Ignoring ok value, this should only fail if an invalid storeType is provided
	store, _ := s.stores[storeType]
	return store
}

func (s *bboltStorage) AgentFlagsStore() types.KVStore {
	return s.getKVStore(storage.AgentFlagsStore)
}

func (s *bboltStorage) AutoupdateErrorsStore() types.KVStore {
	return s.getKVStore(storage.AutoupdateErrorsStore)
}

func (s *bboltStorage) ConfigStore() types.KVStore {
	return s.getKVStore(storage.ConfigStore)
}

func (s *bboltStorage) ControlStore() types.KVStore {
	return s.getKVStore(storage.ControlStore)
}

func (s *bboltStorage) InitialResultsStore() types.KVStore {
	return s.getKVStore(storage.InitialResultsStore)
}

func (s *bboltStorage) ResultLogsStore() types.KVStore {
	return s.getKVStore(storage.ResultLogsStore)
}

func (s *bboltStorage) OsqueryHistoryInstanceStore() types.KVStore {
	return s.getKVStore(storage.OsqueryHistoryInstanceStore)
}

func (s *bboltStorage) SentNotificationsStore() types.KVStore {
	return s.getKVStore(storage.SentNotificationsStore)
}

func (s *bboltStorage) StatusLogsStore() types.KVStore {
	return s.getKVStore(storage.StatusLogsStore)
}

func (s *bboltStorage) ServerProvidedDataStore() types.KVStore {
	return s.getKVStore(storage.ServerProvidedDataStore)
}

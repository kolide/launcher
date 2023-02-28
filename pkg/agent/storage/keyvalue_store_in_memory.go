package storage

import (
	"sync"

	"github.com/go-kit/kit/log"
)

type inMemoryKeyValueStore struct {
	logger log.Logger
	mu     sync.RWMutex
	items  map[string][]byte
}

func NewInMemoryKeyValueStore(logger log.Logger) *inMemoryKeyValueStore {
	s := &inMemoryKeyValueStore{
		logger: logger,
		items:  make(map[string][]byte),
	}

	return s
}

func (s *inMemoryKeyValueStore) Get(key []byte) (value []byte, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.items[string(key)]; ok {
		return v, nil
	}
	return nil, nil
}

func (s *inMemoryKeyValueStore) Set(key, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[string(key)] = value
	return nil
}

func (s *inMemoryKeyValueStore) Delete(key []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, string(key))
	return nil
}

func (s *inMemoryKeyValueStore) ForEach(fn func(k, v []byte) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.items {
		if err := fn([]byte(k), v); err != nil {
			return err
		}
	}
	return nil
}

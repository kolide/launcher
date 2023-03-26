package inmemory

import (
	"errors"
	"sync"

	"github.com/go-kit/kit/log"
)

type inMemoryKeyValueStore struct {
	logger log.Logger
	mu     sync.RWMutex
	items  map[string][]byte
}

func NewStore(logger log.Logger) *inMemoryKeyValueStore {
	s := &inMemoryKeyValueStore{
		logger: logger,
		items:  make(map[string][]byte),
	}

	return s
}

func (s *inMemoryKeyValueStore) Get(key []byte) (value []byte, err error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.items[string(key)]; ok {
		return v, nil
	}
	return nil, nil
}

func (s *inMemoryKeyValueStore) Set(key, value []byte) error {
	if s == nil {
		return errors.New("store is nil")
	}

	if string(key) == "" {
		return errors.New("key is blank")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[string(key)] = value
	return nil
}

func (s *inMemoryKeyValueStore) Delete(keys ...[]byte) error {
	if s == nil {
		return errors.New("store is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.items, string(key))
	}
	return nil
}

func (s *inMemoryKeyValueStore) ForEach(fn func(k, v []byte) error) error {
	if s == nil {
		return errors.New("store is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.items {
		if err := fn([]byte(k), v); err != nil {
			return err
		}
	}
	return nil
}

func (s *inMemoryKeyValueStore) Update(pairs ...string) ([]string, error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kvPairs := make(map[string]string)
	for i := 0; i < len(pairs)-1; i += 2 {
		kvPairs[pairs[i]] = pairs[i+1]
	}

	s.items = make(map[string][]byte)

	for key, value := range kvPairs {
		if key == "" {
			return nil, errors.New("key is blank")
		}

		s.items[key] = []byte(value)
	}

	var deletedKeys []string

	for key, _ := range s.items {
		if _, ok := kvPairs[string(key)]; ok {
			continue
		}

		delete(s.items, string(key))

		// Remember which keys we're deleting
		deletedKeys = append(deletedKeys, string(key))
	}

	return deletedKeys, nil
}

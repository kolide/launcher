package inmemory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (s *inMemoryKeyValueStore) Update(data io.Reader) error {
	if s == nil {
		return errors.New("store is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var kvPairs map[string]string
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	s.items = make(map[string][]byte)

	for key, value := range kvPairs {
		if key == "" {
			return errors.New("key is blank")
		}

		s.items[key] = []byte(value)
	}

	return nil
}

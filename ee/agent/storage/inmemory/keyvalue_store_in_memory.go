package inmemory

import (
	"encoding/binary"
	"errors"
	"sync"
)

type inMemoryKeyValueStore struct {
	mu       sync.RWMutex
	items    map[string][]byte
	order    []string
	sequence uint64
}

func NewStore() *inMemoryKeyValueStore {
	s := &inMemoryKeyValueStore{
		items: make(map[string][]byte),
		order: make([]string, 0),
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

	if _, exists := s.items[string(key)]; !exists {
		s.order = append(s.order, string(key))
	}

	s.items[string(key)] = make([]byte, len(value))
	copy(s.items[string(key)], value)

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
		for i, k := range s.order {
			if k == string(key) {
				s.order = append(s.order[:i], s.order[i+1:]...)
				break
			}
		}
	}

	return nil
}

func (s *inMemoryKeyValueStore) DeleteAll() error {
	if s == nil {
		return errors.New("store is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string][]byte)
	s.order = make([]string, 0)

	return nil
}

func (s *inMemoryKeyValueStore) ForEach(fn func(k, v []byte) error) error {
	if s == nil {
		return errors.New("store is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range s.order {
		if err := fn([]byte(key), s.items[key]); err != nil {
			return err
		}
	}
	return nil
}

// Update adheres to the Updater interface for bulk replacing data in a key/value store.
// Note that this method internally defers all mutating operations to the existing Set/Delete
// functions, so the mutex is not locked here
func (s *inMemoryKeyValueStore) Update(kvPairs map[string]string) ([]string, error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	for key, value := range kvPairs {
		if key == "" {
			return nil, errors.New("key is blank")
		}

		s.Set([]byte(key), []byte(value))
	}

	deletedKeys := make([]string, 0)

	for key := range s.items {
		if _, ok := kvPairs[key]; ok {
			continue
		}

		s.Delete([]byte(key))

		// Remember which keys we're deleting
		deletedKeys = append(deletedKeys, key)
	}

	return deletedKeys, nil
}

func (s *inMemoryKeyValueStore) Count() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.items), nil
}

func (s *inMemoryKeyValueStore) AppendValues(values ...[]byte) error {
	if s == nil {
		return errors.New("unable to append values into uninitialized inmemory db store")
	}

	for _, value := range values {
		s.Set(s.nextSequenceKey(), value)
	}

	return nil
}

func (s *inMemoryKeyValueStore) nextSequenceKey() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence++
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, s.sequence)
	return b
}

package inmemory

import (
	"errors"
	"sync"

	"github.com/kolide/launcher/pkg/agent/types"
)

type inMemoryKeyValueStore struct {
	items    map[string][]byte
	order    []string
	sequence uint64
	mu       sync.RWMutex
}

func NewStore() *inMemoryKeyValueStore {
	return &inMemoryKeyValueStore{
		items: make(map[string][]byte),
		order: make([]string, 0),
	}
}

func (s *inMemoryKeyValueStore) Get(key []byte) ([]byte, error) {
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

	s.setItem(key, value)
	return nil
}

// setItem sets the item in the backing items map by copying the value to a
// new slice
func (s *inMemoryKeyValueStore) setItem(key, value []byte) {
	// Because this takes an array, it is always passed by reference. And, there are some cases where that ends up
	// causing issues. So we generaet a copy. this is not an issue with bbolt backed
	// storage, because that inherently does a serialization to disk.
	s.items[string(key)] = make([]byte, len(value))
	copy(s.items[string(key)], value)
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

func (s *inMemoryKeyValueStore) Update(kvPairs map[string]string) ([]string, error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var deletedKeys []string

	newOrder := make([]string, 0, len(kvPairs))
	for _, key := range s.order {

		// if the key exists in the new kvPairs, then we want to keep it
		// so add it to the new order, and set it in the store
		if value, exists := kvPairs[key]; exists {
			newOrder = append(newOrder, key)
			s.setItem([]byte(key), []byte(value))
			continue
		}

		// if the key does not exists in the new kv pairs
		// delete it
		deletedKeys = append(deletedKeys, key)
		delete(s.items, key)
	}

	// now we want to add any new keys to the store
	for k, v := range kvPairs {
		// if it's new to the store, add it to the order and the items
		if _, exists := s.items[k]; !exists {
			newOrder = append(newOrder, k)
			s.setItem([]byte(k), []byte(v))
		}
	}

	s.order = newOrder
	return deletedKeys, nil
}

func (s *inMemoryKeyValueStore) NextSequence() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence++
	return s.sequence, nil
}

func (s *inMemoryKeyValueStore) Len() int {
	s.mu.RLock()
	defer s.mu.Unlock()
	return len(s.items)
}

type cursor struct {
	store *inMemoryKeyValueStore
	keys  []string
	index int
}

func (c *cursor) First() ([]byte, []byte) {
	if len(c.keys) == 0 {
		return nil, nil
	}
	c.index = 0
	key := c.keys[c.index]
	return []byte(key), c.store.items[key]
}

func (c *cursor) Next() ([]byte, []byte) {
	c.index++
	if c.index >= len(c.keys) {
		return nil, nil
	}
	key := c.keys[c.index]
	return []byte(key), c.store.items[key]
}

func (s *inMemoryKeyValueStore) DoCursor(fn func(types.Cursor) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := &cursor{store: s, keys: s.order, index: -1}
	return fn(cur)
}

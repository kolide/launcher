package storage

import (
	"github.com/go-kit/kit/log"
)

type inMemoryKeyValueStore struct {
	logger log.Logger
}

func NewInMemoryKeyValueStore(logger log.Logger) *bboltKeyValueStore {
	m := &bboltKeyValueStore{
		logger: logger,
	}

	return m
}

func (s *inMemoryKeyValueStore) Get(bucketName string, key []byte) (value []byte, err error) {
	return nil, nil // TODO
}

func (s *inMemoryKeyValueStore) Set(bucketName string, key, value []byte) error {
	return nil // TODO
}

func (s *inMemoryKeyValueStore) Delete(bucketName string, key []byte) error {
	return nil // TODO
}

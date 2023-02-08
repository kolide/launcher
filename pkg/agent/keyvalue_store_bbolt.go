package agent

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"go.etcd.io/bbolt"
)

type bboltKeyValueStore struct {
	logger log.Logger
	db     *bbolt.DB
}

func New(logger log.Logger, db *bbolt.DB) *bboltKeyValueStore {
	m := &bboltKeyValueStore{
		logger: logger,
		db:     db,
	}

	return m
}

func (s *bboltKeyValueStore) Get(key []byte) (value []byte, err error) {
	if err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}

		value = b.Get([]byte(privateEccData))
		return nil
	}); err != nil {
		return nil, err
	}

	return value, err
}

func (s *bboltKeyValueStore) Set(key, value []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		if value != nil {
			if err := b.Put([]byte(key), value); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *bboltKeyValueStore) Delete(key []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}

		err := b.Delete([]byte(privateEccData))
		if err != nil {
			return err
		}

		return nil
	})
}

package agentbbolt

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.etcd.io/bbolt"
)

// NoDbError is an error type that represents a nil bbolt database
type NoDbError struct{}

func (e NoDbError) Error() string {
	return "bbolt db is nil"
}

// NoBucketError is an error type that represents a nonexistent bucket
type NoBucketError struct {
	bucketName string
}

func (e NoBucketError) Error() string {
	return fmt.Sprintf("%s bucket does not exist", e.bucketName)
}
func NewNoBucketError(bucketName string) NoBucketError {
	return NoBucketError{bucketName: bucketName}
}

type bboltKeyValueStore struct {
	logger     log.Logger
	db         *bbolt.DB
	bucketName string
}

func NewStore(logger log.Logger, db *bbolt.DB, bucketName string) (*bboltKeyValueStore, error) {
	err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	m := &bboltKeyValueStore{
		logger:     log.With(logger, "bucket", bucketName),
		db:         db,
		bucketName: bucketName,
	}

	return m, nil
}

func (s *bboltKeyValueStore) Get(key []byte) (value []byte, err error) {
	if s == nil || s.db == nil {
		return nil, NoDbError{}
	}

	if err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		value = b.Get(key)
		return nil
	}); err != nil {
		return nil, err
	}

	return value, err
}

func (s *bboltKeyValueStore) Set(key, value []byte) error {
	if s == nil || s.db == nil {
		return NoDbError{}
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		if value != nil {
			if err := b.Put(key, value); err != nil {
				return fmt.Errorf("error setting %s key: %w", string(key), err)
			}
		}

		return nil
	})
}

func (s *bboltKeyValueStore) Delete(keys ...[]byte) error {
	if s == nil || s.db == nil {
		return NoDbError{}
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		for _, key := range keys {
			err := b.Delete(key)
			if err != nil {
				return fmt.Errorf("error deleting %s key: %w", string(key), err)
			}
		}

		return nil
	})
}

func (s *bboltKeyValueStore) ForEach(fn func(k, v []byte) error) error {
	if s == nil || s.db == nil {
		return NoDbError{}
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		if err := b.ForEach(fn); err != nil {
			return fmt.Errorf("error iterating over keys in bucket: %w", err)
		}

		return nil
	})
}

func (s *bboltKeyValueStore) Update(pairs ...string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, NoDbError{}
	}

	kvPairs := make(map[string]string)
	for i := 0; i < len(pairs)-1; i += 2 {
		kvPairs[pairs[i]] = pairs[i+1]
	}

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		for key, value := range kvPairs {
			if err := b.Put([]byte(key), []byte(value)); err != nil {
				// Log errors but continue processing the remaining key-values
				level.Error(s.logger).Log(
					"msg", "failed to store key-value in bucket",
					"key", key,
					"err", err,
				)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	var deletedKeys []string

	// Now prune stale keys from the bucket
	// This operation requires a new transaction
	err = s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		c := b.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			if _, ok := kvPairs[string(key)]; ok {
				// Key exists in the bucket and kvPairs, move on
				continue
			}

			// Key exists in the bucket but not in kvPairs, delete it
			if err := b.Delete(key); err != nil {
				// Log errors but ignore the failure
				level.Error(s.logger).Log(
					"msg", "failed to remove key from bucket",
					"key", key,
					"err", err,
				)
			}

			// Remember which keys we're deleting
			deletedKeys = append(deletedKeys, string(key))
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return deletedKeys, nil
}

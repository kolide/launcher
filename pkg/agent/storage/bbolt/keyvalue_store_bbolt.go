package agentbbolt

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

const (
	dbTestFileName = "test.db"
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

func (s *bboltKeyValueStore) Update(data io.Reader) error {
	if s == nil || s.db == nil {
		return NoDbError{}
	}

	var kvPairs map[string]string
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		return fmt.Errorf("failed to decode '%s' bucket consumer json: %w", s.bucketName, err)
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
		return err
	}

	// Now prune stale keys from the bucket
	// This operation requires a new transaction
	return s.db.Update(func(tx *bbolt.Tx) error {
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
		}

		return nil
	})
}

func (s *bboltKeyValueStore) NumKeys() (int, error) {
	if s == nil || s.db == nil {
		return 0, NoDbError{}
	}

	var count int
	if err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		count = b.Stats().KeyN
		return nil
	}); err != nil {
		return 0, err
	}

	return count, nil
}

func (s *bboltKeyValueStore) Size() (int64, error) {
	if s == nil || s.db == nil {
		return 0, NoDbError{}
	}

	var dbSize int64
	if err := s.db.View(func(tx *bbolt.Tx) error {
		dbSize = tx.Size()
		return nil
	}); err != nil {
		return 0, err
	}

	return dbSize, nil
}

// SetupDB is used for creating bbolt databases for testing
func SetupDB(t *testing.T) *bbolt.DB {
	// Create a temp directory to hold our bbolt db
	dbDir := t.TempDir()

	// Create database; ensure we clean it up after the test
	db, err := bbolt.Open(filepath.Join(dbDir, dbTestFileName), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}

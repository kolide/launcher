package agentbbolt

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/pkg/traces"
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
	slogger    *slog.Logger
	db         *bbolt.DB
	bucketName string
}

func NewStore(ctx context.Context, slogger *slog.Logger, db *bbolt.DB, bucketName string) (*bboltKeyValueStore, error) {
	_, span := traces.StartSpan(ctx, "bucket_name", bucketName)
	defer span.End()

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
		slogger:    slogger.With("bucket", bucketName),
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

func (s *bboltKeyValueStore) DeleteAll() error {
	if s == nil || s.db == nil {
		return NoDbError{}
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.DeleteBucket([]byte(s.bucketName)); err != nil {
			return fmt.Errorf("deleting bucket: %w", err)
		}

		if _, err := tx.CreateBucketIfNotExists([]byte(s.bucketName)); err != nil {
			return fmt.Errorf("re-creating bucket: %w", err)
		}

		return nil
	})
}

// ForEach provides a read-only iterator for all key-value pairs stored within s.bucketName
// this allows bboltKeyValueStore to adhere to the types.Iterator interface
func (s *bboltKeyValueStore) ForEach(fn func(k, v []byte) error) error {
	if s == nil || s.db == nil {
		return NoDbError{}
	}

	return s.db.View(func(tx *bbolt.Tx) error {
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

func (s *bboltKeyValueStore) Update(kvPairs map[string]string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, NoDbError{}
	}

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		for key, value := range kvPairs {
			if err := b.Put([]byte(key), []byte(value)); err != nil {
				// Log errors but continue processing the remaining key-values
				s.slogger.Log(context.TODO(), slog.LevelError,
					"failed to store key-value in bucket",
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
				s.slogger.Log(context.TODO(), slog.LevelError,
					"failed to remove key from bucket",
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

func (s *bboltKeyValueStore) Count() (int, error) {
	if s == nil || s.db == nil {
		s.slogger.Log(context.TODO(), slog.LevelError, "unable to count uninitialized bbolt storage db")
		return 0, NoDbError{}
	}

	var numKeys int
	if err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		numKeys = b.Stats().KeyN
		return nil
	}); err != nil {
		s.slogger.Log(context.TODO(), slog.LevelError,
			"err counting from bucket",
			"err", err,
		)
		return 0, err
	}

	return numKeys, nil
}

// AppendValues utlizes bbolts NextSequence functionality to add ordered values
// after generating the next autoincrementing key for each
func (s *bboltKeyValueStore) AppendValues(values ...[]byte) error {
	if s == nil || s.db == nil {
		return errors.New("unable to append values into uninitialized bbolt db store")
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(s.bucketName))
		if b == nil {
			return NewNoBucketError(s.bucketName)
		}

		for _, value := range values {
			key, err := b.NextSequence()
			if err != nil {
				return fmt.Errorf("generating key: %w", err)
			}

			if err = b.Put(byteKeyFromUint64(key), value); err != nil {
				return fmt.Errorf("adding ordered value: %w", err)
			}
		}

		return nil
	})
}

func byteKeyFromUint64(k uint64) []byte {
	// Adapted from Bolt docs
	// 8 bytes in a uint64
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, k)
	return b
}

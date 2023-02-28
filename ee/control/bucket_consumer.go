package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.etcd.io/bbolt"
)

// bucketConsumer processes control server updates for a named bucket
type bucketConsumer struct {
	logger log.Logger
	db     *bbolt.DB
	// getset     types.GetterSetter
	bucketName string
}

func NewBucketConsumer(logger log.Logger, db *bbolt.DB, bucketName string) *bucketConsumer {
	bc := &bucketConsumer{
		logger: logger,
		db:     db,
		// getset:     agent.NewBBoltKeyValueStore(logger, db, bucketName),
		bucketName: bucketName,
	}

	return bc
}

func (bc *bucketConsumer) Update(data io.Reader) error {
	if bc == nil || bc.db == nil {
		return errors.New("bucketConsumer is nil")
	}

	var kvPairs map[string]string
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		return fmt.Errorf("failed to decode '%s' bucket consumer json: %w", bc.bucketName, err)
	}

	err := bc.db.Update(func(tx *bbolt.Tx) error {
		// Either create the bucket, or retrieve the existing one
		bucket, err := tx.CreateBucketIfNotExists([]byte(bc.bucketName))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		for key, value := range kvPairs {
			if err := bucket.Put([]byte(key), []byte(value)); err != nil {
				// Log errors but continue processing the remaining key-values
				level.Error(bc.logger).Log(
					"msg", "failed to store key-value in bucket",
					"key", key,
					"value", value,
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
	return bc.db.Update(func(tx *bbolt.Tx) error {
		// Bucket should exist from the previous Update invocation
		bucket := tx.Bucket([]byte(bc.bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket does not exist: %w", err)
		}

		c := bucket.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			if _, ok := kvPairs[string(key)]; ok {
				// Key exists in the bucket and kvPairs, move on
				continue
			}

			// Key exists in the bucket but not in kvPairs, delete it
			if err := bucket.Delete(key); err != nil {
				// Log errors but ignore the failure
				level.Error(bc.logger).Log(
					"msg", "failed to remove key from bucket",
					"key", key,
					"err", err,
				)
			}
		}

		return nil
	})
}

/*

func (bc *bucketConsumer) Get(bucketName string, key []byte) (value []byte, err error) {
	return bc.getset.Get(bucketName, key)
	// if bc == nil {
	// 	return nil, errors.New("bucketConsumer is nil")
	// }

	// if err := bc.db.View(func(tx *bbolt.Tx) error {
	// 	b := tx.Bucket([]byte(bc.bucketName))
	// 	if b == nil {
	// 		return fmt.Errorf("%s bucket not found", bc.bucketName)
	// 	}

	// 	value = b.Get(key)
	// 	return nil
	// }); err != nil {
	// 	return nil, fmt.Errorf("fetching data: %w", err)
	// }

	// return value, nil
}

func (bc *bucketConsumer) Set(bucketName string, key, value []byte) error {
	return bc.getset.Set(bucketName, key, value)
	// if bc == nil {
	// 	return errors.New("bucketConsumer is nil")
	// }

	// return bc.db.Update(func(tx *bbolt.Tx) error {
	// 	// Either create the bucket, or retrieve the existing one
	// 	bucket, err := tx.CreateBucketIfNotExists([]byte(bc.bucketName))
	// 	if err != nil {
	// 		return fmt.Errorf("creating bucket: %w", err)
	// 	}

	// 	if err := bucket.Put(key, value); err != nil {
	// 		return fmt.Errorf("error putting key-value in bucket: %w", err)
	// 	}

	// 	return nil
	// })
}

*/

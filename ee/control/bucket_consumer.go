package control

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.etcd.io/bbolt"
)

// bucketConsumer processes control server updates for a named bucket
type bucketConsumer struct {
	logger     log.Logger
	db         *bbolt.DB
	bucketName string
}

func NewBucketConsumer(logger log.Logger, db *bbolt.DB, bucketName string) *bucketConsumer {
	bc := &bucketConsumer{
		logger:     logger,
		db:         db,
		bucketName: bucketName,
	}

	return bc
}

func (bc *bucketConsumer) Update(data io.Reader) error {
	var kvPairs map[string]string
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		return fmt.Errorf("failed to decode '%s' bucket consumer json: %w", bc.bucketName, err)
	}

	return bc.db.Update(func(tx *bbolt.Tx) error {
		// Either create the bucket, or retrieve the existing one
		bucket, err := tx.CreateBucketIfNotExists([]byte(bc.bucketName))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		for key, value := range kvPairs {
			if value == "" {
				// Interpret empty values as key removals
				if err := bucket.Delete([]byte(key)); err != nil {
					// Log errors but continue processing the remaining key-values
					level.Error(bc.logger).Log(
						"msg", "failed to remove key from bucket",
						"key", key,
						"err", err,
					)
				}

				continue
			}

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
}

package control

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.etcd.io/bbolt"
)

// BucketConsumer processes control server updates for a named bucket
type BucketConsumer struct {
	logger     log.Logger
	db         *bbolt.DB
	bucketName string
}

func NewBucketConsumer(logger log.Logger, db *bbolt.DB, bucketName string) *BucketConsumer {
	bc := &BucketConsumer{
		logger:     logger,
		db:         db,
		bucketName: bucketName,
	}

	return bc
}

func (bc *BucketConsumer) Update(data io.Reader) {
	var kvPairs map[string]string
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		level.Error(bc.logger).Log(
			"msg", "failed to decode bucket consumer json",
			"bucketName", bc.bucketName,
			"data", data,
			"err", err,
		)
	}

	bc.db.Update(func(tx *bbolt.Tx) error {
		// Clear the bucket first
		tx.DeleteBucket([]byte(bc.bucketName))

		bucket, err := tx.CreateBucketIfNotExists([]byte(bc.bucketName))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		for key, value := range kvPairs {
			if err := bucket.Put([]byte(key), []byte(value)); err != nil {
				return fmt.Errorf("storing key: %w", err)
			}
		}

		return nil
	})
}

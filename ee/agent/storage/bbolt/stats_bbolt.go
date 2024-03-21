package agentbbolt

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"go.etcd.io/bbolt"
)

type bucketStatsHolder struct {
	Stats        bbolt.BucketStats
	FillPercent  float64
	NumberOfKeys int
	Size         int
}

type dbStatsHolder struct {
	Stats bbolt.TxStats
	Size  int64
}

type Stats struct {
	DB      dbStatsHolder
	Buckets map[string]bucketStatsHolder
}

type bboltStatStore struct {
	slogger *slog.Logger
	db      *bbolt.DB
}

// NewStatStore provides a wrapper around a bbolt.DB connection. This is done at the global (above bucket) level
// and should be used for any operations regarding the collection of storage statistics
func NewStatStore(slogger *slog.Logger, db *bbolt.DB) (*bboltStatStore, error) {
	if db == nil {
		return nil, NoDbError{}
	}

	s := &bboltStatStore{
		slogger: slogger,
		db:      db,
	}

	return s, nil
}

func (s *bboltStatStore) SizeBytes() (int64, error) {
	if s == nil || s.db == nil {
		return 0, NoDbError{}
	}

	var size int64

	if err := s.db.View(func(tx *bbolt.Tx) error {
		size = tx.Size()
		return nil
	}); err != nil {
		return 0, fmt.Errorf("creating view tx to check size stat: %w", err)
	}

	return size, nil
}

// GetStats returns a json blob containing both global and bucket-level
// statistics. Note that the bucketName set does not impact the output, all buckets
// will be traversed for stats regardless
func (s *bboltStatStore) GetStats() ([]byte, error) {
	if s == nil || s.db == nil {
		return nil, NoDbError{}
	}

	stats := &Stats{
		Buckets: make(map[string]bucketStatsHolder),
	}

	if err := s.db.View(func(tx *bbolt.Tx) error {
		stats.DB.Stats = tx.Stats()
		stats.DB.Size = tx.Size()

		if err := tx.ForEach(bucketStatsFunc(stats)); err != nil {
			return fmt.Errorf("dumping bucket: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("creating view tx: %w", err)
	}

	statsJson, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}

	return statsJson, nil
}

func bucketStatsFunc(stats *Stats) func([]byte, *bbolt.Bucket) error {
	return func(name []byte, b *bbolt.Bucket) error {
		bstats := b.Stats()

		// KeyN is the number of keys
		// LeafAlloc is pretty close the number of bytes used
		stats.Buckets[string(name)] = bucketStatsHolder{
			Stats:        bstats,
			FillPercent:  b.FillPercent,
			NumberOfKeys: bstats.KeyN,
			Size:         bstats.LeafAlloc,
		}

		return nil
	}
}

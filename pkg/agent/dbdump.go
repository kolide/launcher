package agent

import (
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

// KeyN is the number of keys
// LeafAlloc is pretty close the number of bytes uses

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

func GetStats(db *bbolt.DB) (*Stats, error) {
	stats := &Stats{
		Buckets: make(map[string]bucketStatsHolder),
	}

	if err := db.View(func(tx *bbolt.Tx) error {
		stats.DB.Stats = tx.Stats()
		stats.DB.Size = tx.Size()

		if err := tx.ForEach(bucketStatsFunc(stats)); err != nil {
			return errors.Wrap(err, "dumping bucket")
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "creating view tx")
	}

	return stats, nil
}

func bucketStatsFunc(stats *Stats) func([]byte, *bbolt.Bucket) error {
	return func(name []byte, b *bbolt.Bucket) error {
		bstats := b.Stats()
		stats.Buckets[string(name)] = bucketStatsHolder{
			Stats:        bstats,
			FillPercent:  b.FillPercent,
			NumberOfKeys: bstats.KeyN,
			Size:         bstats.LeafAlloc,
		}

		return nil
	}
}

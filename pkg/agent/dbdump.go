package agent

import (
	"github.com/kolide/launcher/pkg/agent/types"
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

func GetStats(store types.KVStore) (*Stats, error) {
	stats := &Stats{
		Buckets: make(map[string]bucketStatsHolder),
	}
	/*
		if err := db.View(func(tx *bbolt.Tx) error {
			stats.DB.Stats = tx.Stats()
			stats.DB.Size = tx.Size()

			if err := tx.ForEach(bucketStatsFunc(stats)); err != nil {
				return fmt.Errorf("dumping bucket: %w", err)
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("creating view tx: %w", err)
		}*/ // TODO Stats

	return stats, nil
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

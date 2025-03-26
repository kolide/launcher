package checkups

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kolide/launcher/ee/agent"
	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	"github.com/kolide/launcher/ee/agent/types"
	"go.etcd.io/bbolt"
)

type bboltdbCheckup struct {
	k    types.Knapsack
	data map[string]any
}

func (c *bboltdbCheckup) Name() string {
	return "bboltdb"
}

func (c *bboltdbCheckup) Run(_ context.Context, extraFH io.Writer) error {
	db := c.k.BboltDB()
	if db == nil {
		// Not an error -- we are probably running standalone instead of in situ
		return nil
	}

	stats, err := agent.GetStats(db)
	if err != nil {
		return fmt.Errorf("getting db stats: %w", err)
	}

	c.data = make(map[string]any)
	for k, v := range stats.Buckets {
		c.data[k] = v
	}

	// Gather additional data only if we're running flare
	if extraFH == io.Discard {
		return nil
	}

	backupStats, err := c.backupStats()
	if err != nil {
		fmt.Fprintf(extraFH, "could not get stats for backup database: %v\n", err)
		return nil
	}

	if err := json.NewEncoder(extraFH).Encode(backupStats); err != nil {
		fmt.Fprintf(extraFH, "could not write stats for backup database: %v\n", err)
		return nil
	}

	return nil
}

func (c *bboltdbCheckup) backupStats() (map[string]map[string]any, error) {
	backupStatsMap := make(map[string]map[string]any)

	backupDbLocations := agentbbolt.BackupLauncherDbLocations(c.k.RootDirectory())

	for _, backupDbLocation := range backupDbLocations {
		if _, err := os.Stat(backupDbLocation); err != nil {
			continue
		}

		backupStatsMap[backupDbLocation] = make(map[string]any)

		backupStats, err := backupStatsFromDb(backupDbLocation)
		if err != nil {
			return nil, fmt.Errorf("could not get backup db stats from %s: %w", backupDbLocation, err)
		}

		for k, v := range backupStats.Buckets {
			backupStatsMap[backupDbLocation][k] = v
		}
	}

	return backupStatsMap, nil
}

func backupStatsFromDb(backupDbLocation string) (*agent.Stats, error) {
	// Open a connection to the backup, since we don't have one available yet
	boltOptions := &bbolt.Options{Timeout: time.Duration(30) * time.Second}
	backupDb, err := bbolt.Open(backupDbLocation, 0600, boltOptions)
	if err != nil {
		return nil, fmt.Errorf("could not open backup db at %s: %w", backupDbLocation, err)
	}
	defer backupDb.Close()

	// Gather stats
	backupStats, err := agent.GetStats(backupDb)
	if err != nil {
		return nil, fmt.Errorf("could not get backup db stats: %w", err)
	}

	return backupStats, nil
}

func (c *bboltdbCheckup) ExtraFileName() string {
	return "backup.json"
}

func (c *bboltdbCheckup) Status() Status {
	return Informational
}

func (c *bboltdbCheckup) Summary() string {
	return "N/A"
}

func (c *bboltdbCheckup) Data() any {
	return c.data
}

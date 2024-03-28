package checkups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
)

type kvStorageStatsCheckup struct {
	k    types.Knapsack
	data map[string]any
}

func (c *kvStorageStatsCheckup) Name() string {
	return "KV Storage Stats"
}

func (c *kvStorageStatsCheckup) Run(_ context.Context, _ io.Writer) error {
	db := c.k.StorageStatTracker()
	if db == nil {
		return errors.New("no db connection available for storage stat tracking")
	}

	stats, err := db.GetStats()
	if err != nil {
		return fmt.Errorf("getting db stats: %w", err)
	}

	data := make(map[string]any)

	if err := json.Unmarshal(stats, &data); err != nil {
		return fmt.Errorf("unmarshalling storage stats json: %w", err)
	}

	c.data = data

	return nil
}

func (c *kvStorageStatsCheckup) ExtraFileName() string {
	return ""
}

func (c *kvStorageStatsCheckup) Status() Status {
	return Informational
}

func (c *kvStorageStatsCheckup) Summary() string {
	return "N/A"
}

func (c *kvStorageStatsCheckup) Data() any {
	return c.data
}

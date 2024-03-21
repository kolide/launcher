package checkups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
)

type bboltdbCheckup struct {
	k    types.Knapsack
	data map[string]any
}

func (c *bboltdbCheckup) Name() string {
	return "bboltdb"
}

func (c *bboltdbCheckup) Run(_ context.Context, _ io.Writer) error {
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

func (c *bboltdbCheckup) ExtraFileName() string {
	return ""
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

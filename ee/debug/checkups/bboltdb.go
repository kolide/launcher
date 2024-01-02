package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/agent"
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
	db := c.k.BboltDB()
	if db == nil {
		return errors.New("no DB available")
	}

	stats, err := agent.GetStats(db)
	if err != nil {
		return fmt.Errorf("getting db stats: %w", err)
	}

	c.data = make(map[string]any)
	for k, v := range stats.Buckets {
		c.data[k] = v
	}

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

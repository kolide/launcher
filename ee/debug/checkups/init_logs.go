package checkups

import (
	"archive/zip"
	"context"
	"io"

	"github.com/kolide/launcher/v2/ee/agent/types"
)

// The maximum number of log lines to retrieve
const maxInitLogLines = 10000 // nolint:unused

type InitLogs struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (c *InitLogs) Name() string {
	return "Init Logs"
}

func (c *InitLogs) Run(ctx context.Context, fullFH io.Writer) error {
	c.status = Passing

	// if were discarding, just return
	if fullFH == io.Discard {
		return nil
	}

	logZip := zip.NewWriter(fullFH)
	defer logZip.Close()

	return writeInitLogs(ctx, c.k, logZip)
}

func (c *InitLogs) Status() Status {
	return c.status
}

func (c *InitLogs) Summary() string {
	return c.summary
}

func (c *InitLogs) ExtraFileName() string {
	return "init_logs.zip"
}

func (c *InitLogs) Data() any {
	return nil
}

package checkups

import (
	"archive/zip"
	"context"
	"io"
)

type InitLogs struct {
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

	return writeInitLogs(ctx, logZip)
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

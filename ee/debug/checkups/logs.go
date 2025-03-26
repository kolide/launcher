package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kolide/launcher/ee/agent/types"
)

type Logs struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (c *Logs) Name() string {
	return "Logs"
}

func (c *Logs) Run(_ context.Context, fullFH io.Writer) error {

	debugLog := filepath.Join(c.k.RootDirectory(), "debug.json")

	stat, err := os.Stat(debugLog)
	if err != nil {
		// Failure to stat logs isn't really a checkup error -- it's probably indicative of no logs or no permission,
		// or similar. Which is more of a failed check, then a validity error with the checkup.
		c.status = Failing
		c.summary = fmt.Sprintf("failed to stat debug log: %s", err)
		return nil
	}

	c.status = Passing
	c.summary = fmt.Sprintf("debug.json is %d bytes, and was last modified at %s", stat.Size(), stat.ModTime())

	// Now, if we're not going to discard it, copy the logs
	if fullFH == io.Discard {
		return nil
	}

	logZip := zip.NewWriter(fullFH)
	defer logZip.Close()

	matches, _ := filepath.Glob(filepath.Join(c.k.RootDirectory(), "debug*"))

	for _, f := range matches {
		if err := addFileToZip(logZip, f); err != nil {
			return fmt.Errorf("adding %s to zip: %w", f, err)
		}
	}

	return nil

}

func (c *Logs) Status() Status {
	return c.status
}

func (c *Logs) Summary() string {
	return c.summary
}

func (c *Logs) ExtraFileName() string {
	return "logs.zip"
}

func (c *Logs) Data() any {
	return nil
}

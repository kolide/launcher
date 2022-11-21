package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

func runLauncher(ctx context.Context, cancel func(), args []string, logger log.Logger) error {
	count := 0

	for {
		count = count + 1
		level.Debug(logger).Log(
			"msg", "Launcher Loop",
			"count", count,
			"pid", ProcessNotes.Pid,
			"size", ProcessNotes.Size,
			"modtime", ProcessNotes.ModTime,
		)
		time.Sleep(5 * time.Second)

		if count > 4 {
			if err := triggerUpgrade(ctx, cancel, logger); err != nil {
				return fmt.Errorf("triggerUpgrade: %w", err)
			}
			break
		}
	}

	level.Debug(logger).Log("msg", "I guess we're exiting", "pid", ProcessNotes.Pid)
	cancel()
	return nil
}

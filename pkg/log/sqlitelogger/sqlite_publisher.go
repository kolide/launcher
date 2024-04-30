package sqlitelogger

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

type (
	SqliteLogPublisher struct {
		slogger     *slog.Logger
		interrupt   chan struct{}
		interrupted bool
		publisher   types.LogStore
	}
)

func NewSqliteLogPublisher(slogger *slog.Logger, logPublisher types.LogStore) *SqliteLogPublisher {
	return &SqliteLogPublisher{
		slogger:   slogger.With("component", "sqlite_log_publisher"),
		interrupt: make(chan struct{}, 1),
		publisher: logPublisher,
	}
}

// Run starts a log publication routine. The purpose of this is to
// pull logs out of the sqlite database and write them to debug.json so we can
// use all of the existing log publication and cleanup logic while maintaining a single writer
func (slp *SqliteLogPublisher) Run() error {
	ctx := context.TODO()
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		slp.Once(ctx)

		select {
		case <-ticker.C:
			continue
		case <-slp.interrupt:
			slp.slogger.Log(ctx, slog.LevelDebug,
				"interrupt received, exiting execute loop",
			)
			return nil
		}
	}
}

func (slp *SqliteLogPublisher) Once(ctx context.Context) {
	logsToDelete := make([]any, 0)

	if err := slp.publisher.ForEach(func(rowid, timestamp int64, v []byte) error {
		logRecord := make(map[string]any)
		if err := json.Unmarshal(v, &logRecord); err != nil {
			slp.slogger.Log(ctx, slog.LevelError, "failed to unmarshal sqlite log", "log", string(v))
			logsToDelete = append(logsToDelete, rowid)
			// log the issue but don't return an error, we want to keep processing whatever we can
			return nil
		}

		logArgs := make([]slog.Attr, len(logRecord))
		for k, v := range logRecord {
			logArgs = append(logArgs, slog.Any(k, v))
		}

		// re-issue the log, this time with the debug.json writer
		// pulling out the existing log and re-adding all attributes like this will overwrite
		// the automatic timestamp creation, as well as the msg and level set below
		slp.slogger.LogAttrs(ctx, slog.LevelInfo, "", logArgs...)
		logsToDelete = append(logsToDelete, rowid)

		return nil
	}); err != nil {
		slp.slogger.Log(ctx, slog.LevelError, "iterating sqlite logs", "err", err)
		return
	}

	slp.slogger.Log(ctx, slog.LevelInfo, "collected logs for deletion", "rowids", logsToDelete)

	if err := slp.publisher.DeleteRows(logsToDelete...); err != nil {
		slp.slogger.Log(ctx, slog.LevelError, "cleaning up published sqlite logs", "err", err)
	}
}

func (slp *SqliteLogPublisher) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if slp.interrupted {
		return
	}

	slp.interrupted = true
	slp.interrupt <- struct{}{}
}

package sqlitelogger

import (
	"context"
	"fmt"

	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

type (
	// RestartServiceLogWriter adheres to the io.Writer interface
	SqliteLogWriter struct {
		writer types.TimestampedIteratorAppenderCounterCloser
	}
)

func NewSqliteLogWriter(ctx context.Context, rootDirectory string, tableName agentsqlite.StoreName) (*SqliteLogWriter, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	writer, err := agentsqlite.OpenRW(ctx, rootDirectory, tableName)
	if err != nil {
		return nil, fmt.Errorf("opening log db in %s: %w", rootDirectory, err)
	}

	slw := &SqliteLogWriter{writer: writer}

	return slw, nil
}

func (s *SqliteLogWriter) Close() error {
	if s.writer != nil {
		return s.writer.Close()
	}

	return nil
}

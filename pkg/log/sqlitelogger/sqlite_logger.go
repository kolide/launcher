package sqlitelogger

import (
	"context"
	"fmt"
	"time"

	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

type (
	// RestartServiceLogWriter wraps a sqlite write connection and
	// implements the io.Writer interface, allowing sqlite to be used as a logging backend
	// when used as a multislogger handler
	SqliteLogWriter struct {
		writer types.LogStore
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

// Write implements the io.Writer interface
func (s *SqliteLogWriter) Write(p []byte) (n int, err error) {
	timestamp := time.Now().Unix()
	if err := s.writer.AppendValue(timestamp, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (s *SqliteLogWriter) Close() error {
	if s.writer != nil {
		return s.writer.Close()
	}

	return nil
}

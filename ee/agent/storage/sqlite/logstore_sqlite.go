package agentsqlite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

func (s *sqliteStore) AppendValue(timestamp int64, value []byte) error {
	colInfo := s.getColumns()
	if s == nil || s.conn == nil || colInfo == nil {
		return errors.New("store is nil")
	}

	if s.readOnly {
		return errors.New("cannot perform update with RO connection")
	}

	if !colInfo.isLogstore {
		return errors.New("this table type does not support adding values by timestamp")
	}

	insertSql := fmt.Sprintf(
		`INSERT INTO %s (%s, %s) VALUES (?, ?)`,
		s.tableName,
		colInfo.pk,
		colInfo.valueColumn,
	)

	if _, err := s.conn.Exec(insertSql, timestamp, value); err != nil {
		return fmt.Errorf("appending row into %s: %w", s.tableName, err)
	}

	return nil
}

func (s *sqliteStore) DeleteRows(rowids ...any) error {
	if s == nil || s.conn == nil {
		return errors.New("store is nil")
	}

	if s.readOnly {
		return errors.New("cannot perform deletes with RO connection")
	}

	if len(rowids) == 0 {
		return nil
	}

	// interpolate the proper number of question marks
	paramQs := strings.Repeat("?,", len(rowids))
	paramQs = paramQs[:len(paramQs)-1]
	deleteSql := fmt.Sprintf(`DELETE FROM %s WHERE rowid IN (%s)`, s.tableName, paramQs)

	if _, err := s.conn.Exec(deleteSql, rowids...); err != nil {
		return fmt.Errorf("deleting row from %s: %w", s.tableName, err)
	}

	return nil
}

func (s *sqliteStore) ForEach(fn func(rowid, timestamp int64, v []byte) error) error {
	colInfo := s.getColumns()
	if s == nil || s.conn == nil || colInfo == nil {
		return errors.New("store is nil")
	}

	if !colInfo.isLogstore {
		return errors.New("this table type is not supported for timestamped iteration")
	}

	query := fmt.Sprintf(
		`SELECT rowid, %s, %s FROM %s;`,
		colInfo.pk,
		colInfo.valueColumn,
		s.tableName,
	)

	rows, err := s.conn.Query(query)
	if err != nil {
		return fmt.Errorf("issuing foreach query: %w", err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			s.slogger.Log(context.TODO(), slog.LevelWarn,
				"closing rows after scanning results",
				"err", err,
			)
		}
		if err := rows.Err(); err != nil {
			s.slogger.Log(context.TODO(), slog.LevelWarn,
				"encountered iteration error",
				"err", err,
			)
		}
	}()

	for rows.Next() {
		var rowid int64
		var timestamp int64
		var result string
		if err := rows.Scan(&rowid, &timestamp, &result); err != nil {
			return fmt.Errorf("scanning foreach query: %w", err)
		}

		if err := fn(rowid, timestamp, []byte(result)); err != nil {
			return fmt.Errorf("caller error during foreach iteration: %w", err)
		}
	}

	return nil
}

// Write implements the io.Writer interface, allowing sqliteStore to be used as
// a logging backend via multislogger handler
func (s *sqliteStore) Write(p []byte) (n int, err error) {
	if s.readOnly {
		return 0, errors.New("cannot perform write with RO connection")
	}

	colInfo := s.getColumns()
	if s == nil || s.conn == nil || colInfo == nil {
		return 0, errors.New("store is nil")
	}

	if !colInfo.isLogstore {
		return 0, errors.New("this table type is not supported for timestamped logging")
	}

	timestamp := time.Now().Unix()
	if err := s.AppendValue(timestamp, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

package agentsqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

func (s *sqliteStore) FetchResults(ctx context.Context, columnName string) ([][]byte, error) {
	results := make([][]byte, 0)

	if s == nil || s.conn == nil {
		return results, errors.New("store is nil")
	}

	// It's fine to interpolate the table name into the query because we allowlist via `storeName` type
	query := fmt.Sprintf(`SELECT timestamp, ? FROM %s;`, s.tableName)
	rows, err := s.conn.QueryContext(ctx, query, columnName)
	if err != nil {
		return results, err
	}

	defer rows.Close()

	for rows.Next() {
		var timestamp int64
		var result string
		if err := rows.Scan(&timestamp, &result); err != nil {
			return results, err
		}
		results = append(results, []byte(result))
	}

	return results, nil
}

func (s *sqliteStore) FetchLatestResult(ctx context.Context, columnName string) ([]byte, error) {
	if s == nil || s.conn == nil {
		return []byte{}, errors.New("store is nil")
	}

	// It's fine to interpolate the table name into the query because we allowlist via `storeName` type
	query := fmt.Sprintf(`SELECT timestamp, ? FROM %s ORDER BY timestamp DESC LIMIT 1;`, s.tableName)
	var timestamp int64
	var result string

	err := s.conn.QueryRowContext(ctx, query, columnName).Scan(&timestamp, &result)
	switch {
	case err == sql.ErrNoRows:
		return []byte{}, nil
	case err != nil:
		return []byte{}, err
	default:
		return []byte(result), nil
	}
}

func (s *sqliteStore) AddResult(ctx context.Context, timestamp int64, result []byte) error {
	if s == nil || s.conn == nil {
		return errors.New("store is nil")
	}

	if s.readOnly {
		return errors.New("cannot perform AddResult with RO connection")
	}

	// It's fine to interpolate the table name into the query because we allowlist via `storeName` type
	insertSql := fmt.Sprintf(`INSERT INTO %s (timestamp, results) VALUES (?, ?);`, s.tableName)

	if _, err := s.conn.Exec(insertSql, timestamp, string(result)); err != nil {
		return fmt.Errorf("inserting into %s: %w", s.tableName, err)
	}

	return nil
}
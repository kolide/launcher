package agentsqlite

import (
	"errors"
	"fmt"
	"strings"
)

func (s *sqliteStore) getColumns() *sqliteColumns {
	switch s.tableName {
	case StartupSettingsStore.String():
		return &sqliteColumns{pk: "name", valueColumn: "value"}
	case WatchdogLogStore.String():
		return &sqliteColumns{pk: "timestamp", valueColumn: "log"}
	}

	return nil
}

func (s *sqliteStore) AppendValue(timestamp int64, value []byte) error {
	colInfo := s.getColumns()
	if s == nil || s.conn == nil || colInfo == nil {
		return errors.New("store is nil")
	}

	if s.readOnly {
		return errors.New("cannot perform update with RO connection")
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

	query := fmt.Sprintf(
		`SELECT rowid, %s, %s  FROM %s;`,
		colInfo.pk,
		colInfo.valueColumn,
		s.tableName,
	)

	rows, err := s.conn.Query(query)
	if err != nil {
		return fmt.Errorf("issuing foreach query: %w", err)
	}

	defer rows.Close()

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

func (s *sqliteStore) Count() (int, error) {
	if s == nil || s.conn == nil {
		return 0, errors.New("store is nil")
	}

	// It's fine to interpolate the table name into the query because
	// we require the table name to be in our allowlist `supportedTables`
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s;`, s.tableName)

	var countValue int
	if err := s.conn.QueryRow(query).Scan(&countValue); err != nil {
		return 0, fmt.Errorf("querying for %s table count: %w", s.tableName, err)
	}

	return countValue, nil
}

package agentsqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sqlite
var migrations embed.FS

type SqliteStore struct {
	conn          *sql.DB
	rootDirectory string
}

func NewStore(ctx context.Context, rootDirectory string) (*SqliteStore, error) {
	conn, err := dbConn(ctx, rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", rootDirectory, err)
	}

	s := &SqliteStore{
		conn:          conn,
		rootDirectory: rootDirectory,
	}

	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrating the database: %w", err)
	}

	return s, nil
}

// dbConn returns a connection to the database in the given rootDirectory.
// It will create a database there if one does not yet exist.
func dbConn(ctx context.Context, rootDirectory string) (*sql.DB, error) {
	// Ensure db exists
	startupDbFilepath := dbLocation(rootDirectory)
	if _, err := os.Stat(startupDbFilepath); os.IsNotExist(err) {
		f, err := os.Create(startupDbFilepath)
		if err != nil {
			return nil, fmt.Errorf("creating file at %s: %w", startupDbFilepath, err)
		}
		f.Close()
	}

	// Open and validate connection
	conn, err := sql.Open("sqlite", dbLocation(rootDirectory))
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", rootDirectory, err)
	}
	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("establishing valid connection to startup db: ping error: %w", err)
	}

	return conn, nil
}

// dbLocation standardizes the filepath to the given database.
func dbLocation(rootDirectory string) string {
	// Note that the migration framework expects a net/url style path,
	// so we adjust the rootDirectory with filepath.ToSlash and then
	// use path.Join instead of filepath.Join here.
	return path.Join(filepath.ToSlash(rootDirectory), "startup.db")
}

// migrate makes sure that the database schema is correct.
func (s *SqliteStore) migrate(ctx context.Context) error {
	d, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("loading migration files: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite://%s", dbLocation(s.rootDirectory)))
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}

	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

func (s *SqliteStore) Close() error {
	return s.conn.Close()
}

func (s *SqliteStore) Get(key []byte) (value []byte, err error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	var keyValue string
	if err := s.conn.QueryRow(`SELECT key_value FROM keyvalue_pairs WHERE key_name = ?;`, string(key)).Scan(&keyValue); err != nil {
		return nil, fmt.Errorf("querying flag value: %w", err)
	}
	return []byte(keyValue), nil
}

func (s *SqliteStore) Set(key, value []byte) error {
	if s == nil {
		return errors.New("store is nil")
	}

	if string(key) == "" {
		return errors.New("key is blank")
	}

	upsertSql := `
INSERT INTO keyvalue_pairs (key_name, key_value)
VALUES (?, ?)
ON CONFLICT (key_name) DO UPDATE SET key_value=excluded.key_value;`

	if _, err := s.conn.Exec(upsertSql, string(key), string(value)); err != nil {
		return fmt.Errorf("upserting into keyvalue_pairs: %w", err)
	}

	return nil
}

func (s *SqliteStore) Update(kvPairs map[string]string) ([]string, error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	if len(kvPairs) == 0 {
		return []string{}, nil
	}

	// Wrap in a single transaction
	var err error
	var tx *sql.Tx
	tx, err = s.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}

	// First, perform upsert to for all new and existing keys
	upsertSql := `
INSERT INTO keyvalue_pairs (key_name, key_value)
VALUES %s
ON CONFLICT (key_name) DO UPDATE SET key_value=excluded.key_value;`
	valueStr := strings.TrimRight(strings.Repeat("(?, ?),", len(kvPairs)), ",")

	// Build value args; save key names at the same time to determine which keys to prune later
	valueArgs := make([]any, 2*len(kvPairs))
	keyNames := make([]any, len(kvPairs))
	i := 0
	for k, v := range kvPairs {
		valueArgs[i] = k
		valueArgs[i+1] = v
		keyNames[i/2] = k
		i += 2
	}

	if _, err := tx.Exec(fmt.Sprintf(upsertSql, valueStr), valueArgs...); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("upserting into keyvalue_pairs: %w; rollback error %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("upserting into keyvalue_pairs: %w", err)
	}

	// Now, prune all keys that must be deleted
	deleteSql := `DELETE FROM keyvalue_pairs WHERE key_name NOT IN (%s) RETURNING key_name;`
	inStr := strings.TrimRight(strings.Repeat("?,", len(keyNames)), ",")

	rows, err := tx.Query(fmt.Sprintf(deleteSql, inStr), keyNames...)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("deleting keys: %w; rollback error %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("deleting keys: %w", err)
	}
	defer rows.Close()

	deletedKeys := make([]string, 0)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return nil, fmt.Errorf("scanning deleted key: %w; rollback error %v", err, rollbackErr)
			}
			return nil, fmt.Errorf("scanning deleted key: %w", err)
		}
		deletedKeys = append(deletedKeys, k)
	}

	// All done -- commit changes
	if err := tx.Commit(); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("committing transaction: %w; rollback error %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return deletedKeys, nil
}

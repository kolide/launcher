package agentsqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	sqlitemigrationdriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

type storeName int

const (
	StartupSettingsStore storeName = iota
	WatchdogLogStore     storeName = 1
)

var missingMigrationErrFormat = regexp.MustCompile(`no migration found for version \d+`)

// String translates the exported int constant to the actual name of the
// supported table in the sqlite database.
func (s storeName) String() string {
	switch s {
	case StartupSettingsStore:
		return "startup_settings"
	case WatchdogLogStore:
		return "watchdog_logs"
	}

	return ""
}

//go:embed migrations/*.sqlite
var migrations embed.FS

type sqliteStore struct {
	slogger       *slog.Logger
	conn          *sql.DB
	readOnly      bool
	rootDirectory string
	tableName     string
}

type sqliteColumns struct {
	pk          string
	valueColumn string
	// isLogstore is used to determine whether the underlying table can support our LogStore interface methods.
	// because any logstore iteration must scan the values into known types, we use this to avoid pulling in
	// the reflect package here and making this more complicated than it needs to be
	isLogstore bool
}

// OpenRO opens a connection to the database in the given root directory; it does
// not perform database creation or migration.
func OpenRO(ctx context.Context, slogger *slog.Logger, rootDirectory string, name storeName) (*sqliteStore, error) {
	if name.String() == "" {
		return nil, fmt.Errorf("unsupported table %d", name)
	}

	conn, err := sql.Open("sqlite", dbLocation(rootDirectory))
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", rootDirectory, err)
	}

	return &sqliteStore{
		slogger:       slogger.With("component", "keyvalue_store_sqlite", "table_name", name.String()),
		conn:          conn,
		readOnly:      true,
		rootDirectory: rootDirectory,
		tableName:     name.String(),
	}, nil
}

// OpenRW creates a validated database connection to a validated database, performing
// migrations if necessary.
func OpenRW(ctx context.Context, rootDirectory string, name storeName) (*sqliteStore, error) {
	if name.String() == "" {
		return nil, fmt.Errorf("unsupported table %d", name)
	}

	conn, err := validatedDbConn(ctx, rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", rootDirectory, err)
	}

	s := &sqliteStore{
		conn:          conn,
		readOnly:      false,
		rootDirectory: rootDirectory,
		tableName:     name.String(),
	}

	if err := s.migrate(); err != nil {
		s.Close()
		return nil, fmt.Errorf("migrating the database: %w", err)
	}

	return s, nil
}

// validatedDbConn returns a connection to the database in the given rootDirectory.
// It will create a database there if one does not yet exist.
func validatedDbConn(ctx context.Context, rootDirectory string) (*sql.DB, error) {
	startupDbFilepath := dbLocation(rootDirectory)
	if err := validateDb(ctx, startupDbFilepath); err != nil {
		// Delete and re-create the database file
		_ = os.Remove(startupDbFilepath)
		f, err := os.Create(startupDbFilepath)
		if err != nil {
			return nil, fmt.Errorf("creating file at %s: %w", startupDbFilepath, err)
		}
		f.Close()
	}

	// Open and validate connection
	conn, err := sql.Open("sqlite", startupDbFilepath)
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", rootDirectory, err)
	}
	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("establishing valid connection to startup db: ping error: %w", err)
	}

	return conn, nil
}

// validateDb ensures that the database file exists and is not corrupt.
func validateDb(ctx context.Context, dbFilepath string) error {
	// Make sure the database exists
	if _, err := os.Stat(dbFilepath); os.IsNotExist(err) {
		return fmt.Errorf("db does not exist at %s", dbFilepath)
	} else if err != nil {
		return fmt.Errorf("determining if db exists: %w", err)
	}

	conn, err := sql.Open("sqlite", dbFilepath)
	if err != nil {
		return fmt.Errorf("creating connection: %w", err)
	}
	defer conn.Close()

	// Make sure the database is valid
	var quickCheckResults string
	if err := conn.QueryRowContext(ctx, `pragma quick_check;`).Scan(&quickCheckResults); err != nil {
		return fmt.Errorf("running quick check: %w", err)
	}
	if quickCheckResults != "ok" {
		return fmt.Errorf("quick check did not pass: %s", quickCheckResults)
	}

	return nil
}

// dbLocation standardizes the filepath to the given database.
func dbLocation(rootDirectory string) string {
	// Note that the migration framework expects a net/url style path,
	// so we adjust the rootDirectory with filepath.ToSlash and then
	// use path.Join instead of filepath.Join here.
	return path.Join(filepath.ToSlash(rootDirectory), "kv.sqlite")
}

// migrate makes sure that the database schema is correct.
func (s *sqliteStore) migrate() error {
	d, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("loading migration files: %w", err)
	}
	defer d.Close()

	dbInstance, err := sqlitemigrationdriver.WithInstance(s.conn, &sqlitemigrationdriver.Config{})
	if err != nil {
		return fmt.Errorf("creating db migration instance: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", d, "sqlite", dbInstance)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}

	// don't prevent DB access for a missing migration, this is the result of a downgrade after previously
	// running a migration
	if err := m.Up(); err != nil {
		// Not actually errors for us -- we're in a successful state
		if errors.Is(err, migrate.ErrNoChange) || isMissingMigrationError(err) {
			return nil
		}

		// If we need to force, do that
		if errDirty, ok := err.(migrate.ErrDirty); ok {
			if err := m.Force(errDirty.Version); err != nil {
				return fmt.Errorf("forcing migration version %d: %w", errDirty.Version, err)
			}
			return nil
		}

		// Some other error -- return it
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

func (s *sqliteStore) Close() error {
	return s.conn.Close()
}

func (s *sqliteStore) Get(key []byte) (value []byte, err error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	// It's fine to interpolate the table name into the query because
	// we require the table name to be in our allowlist `supportedTables`
	query := fmt.Sprintf(`SELECT value FROM %s WHERE name = ?;`, s.tableName)

	var keyValue string
	if err := s.conn.QueryRow(query, string(key)).Scan(&keyValue); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying key `%s`: %w", string(key), err)
	}
	return []byte(keyValue), nil
}

func (s *sqliteStore) Set(key, value []byte) error {
	if s == nil {
		return errors.New("store is nil")
	}

	if s.readOnly {
		return errors.New("cannot perform set with RO connection")
	}

	if string(key) == "" {
		return errors.New("key is blank")
	}

	// It's fine to interpolate the table name into the query because
	// we require the table name to be in our allowlist `supportedTables`
	upsertSql := fmt.Sprintf(`
INSERT INTO %s (name, value)
VALUES (?, ?)
ON CONFLICT (name) DO UPDATE SET value=excluded.value;`,
		s.tableName,
	)

	if _, err := s.conn.Exec(upsertSql, string(key), string(value)); err != nil {
		return fmt.Errorf("upserting into %s: %w", s.tableName, err)
	}

	return nil
}

func (s *sqliteStore) Update(kvPairs map[string]string) ([]string, error) {
	if s == nil {
		return nil, errors.New("store is nil")
	}

	if s.readOnly {
		return nil, errors.New("cannot perform update with RO connection")
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

	// First, perform upsert to for all new and existing keys.

	upsertSql := `
INSERT INTO %s (name, value)
VALUES %s
ON CONFLICT (name) DO UPDATE SET value=excluded.value;`
	valueStr := strings.TrimRight(strings.Repeat("(?, ?),", len(kvPairs)), ",")

	// make sure we don't go over max int size
	// this is driven codeql code scanning
	// https://codeql.github.com/codeql-query-help/go/go-allocation-size-overflow/
	if len(kvPairs) > math.MaxInt/2 {
		return nil, errors.New("too many key-value pairs")
	}

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

	// It's fine to interpolate the table name into the query because
	// we require the table name to be in our allowlist `supportedTables`.
	if _, err := tx.Exec(fmt.Sprintf(upsertSql, s.tableName, valueStr), valueArgs...); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("upserting into %s: %w; rollback error %v", s.tableName, err, rollbackErr)
		}
		return nil, fmt.Errorf("upserting into %s: %w", s.tableName, err)
	}

	// Now, prune all keys that must be deleted
	deleteSql := `DELETE FROM %s WHERE name NOT IN (%s) RETURNING name;`
	inStr := strings.TrimRight(strings.Repeat("?,", len(keyNames)), ",")

	rows, err := tx.Query(fmt.Sprintf(deleteSql, s.tableName, inStr), keyNames...)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("deleting keys: %w; rollback error %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("deleting keys: %w", err)
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

func (s *sqliteStore) getColumns() *sqliteColumns {
	switch s.tableName {
	case StartupSettingsStore.String():
		return &sqliteColumns{pk: "name", valueColumn: "value", isLogstore: false}
	case WatchdogLogStore.String():
		return &sqliteColumns{pk: "timestamp", valueColumn: "log", isLogstore: true}
	}

	return nil
}

func isMissingMigrationError(err error) bool {
	return missingMigrationErrFormat.MatchString(err.Error())
}

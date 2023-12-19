// Package startup provides access to and manages storage of startup data:
// flags/config values/settings that launcher needs during initialization,
// before the knapsack is available.
package startup

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sqlite
var migrations embed.FS

// GetStartupValue retrieves the value for the given flagKey from the startup database
// located in the given rootDirectory.
func GetStartupValue(ctx context.Context, rootDirectory string, flagKey string) (string, error) {
	conn, err := dbConn(ctx, rootDirectory)
	if err != nil {
		return "", fmt.Errorf("getting db connection to fetch startup value: %w", err)
	}
	defer conn.Close()

	var flagValue string
	if err := conn.QueryRowContext(ctx, `SELECT flag_value FROM startup_flag WHERE flag_name = ?;`, flagKey).Scan(&flagValue); err != nil {
		return "", fmt.Errorf("querying flag value: %w", err)
	}

	return flagValue, nil
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
	return filepath.Join(rootDirectory, "startup.db")
}

// startupDatabase records agent flags and their current values,
// responding to updates as a types.FlagsChangeObserver
type startupDatabase struct {
	conn        *sql.DB
	knapsack    types.Knapsack
	storedFlags map[keys.FlagKey]func() any // maps the agent flags to their knapsack getter functions
}

// NewStartupDatabase returns a new startup database, creating and initializing
// the database if necessary.
func NewStartupDatabase(ctx context.Context, knapsack types.Knapsack) (*startupDatabase, error) {
	conn, err := dbConn(ctx, knapsack.RootDirectory())
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", knapsack.RootDirectory(), err)
	}

	s := &startupDatabase{
		conn:     conn,
		knapsack: knapsack,
		storedFlags: map[keys.FlagKey]func() any{
			keys.UpdateChannel: func() any { return knapsack.UpdateChannel() },
		},
	}

	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrating the database: %w", err)
	}

	// Attempt to ensure flags are up-to-date
	if err := s.setFlags(ctx); err != nil {
		s.knapsack.Slogger().Log(ctx, slog.LevelWarn, "could not set flags", "err", err)
	}

	s.knapsack.RegisterChangeObserver(s, keys.UpdateChannel)

	return s, nil
}

// migrate makes sure that the database schema is correct.
func (s *startupDatabase) migrate(ctx context.Context) error {
	d, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("loading migration files: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite://%s", dbLocation(s.knapsack.RootDirectory())))
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

// setFlags updates the flags with their values from the agent flag data store.
func (s *startupDatabase) setFlags(ctx context.Context) error {
	upsertSql := `
INSERT INTO startup_flag (flag_name, flag_value)
VALUES %s
ON CONFLICT (flag_name) DO UPDATE SET flag_value=excluded.flag_value;
	`
	valueStr := strings.TrimRight(strings.Repeat("(?, ?),", len(s.storedFlags)), ",")

	valueArgs := make([]any, 2*len(s.storedFlags))
	i := 0
	for flag, getter := range s.storedFlags {
		valueArgs[i] = flag.String()
		valueArgs[i+1] = getter()
		i += 2
	}

	if _, err := s.conn.ExecContext(ctx, fmt.Sprintf(upsertSql, valueStr), valueArgs...); err != nil {
		return fmt.Errorf("upserting into startup_flags: %w", err)
	}

	return nil
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface. When a flag
// that the startup database is registered for has a new value, the startup database
// stores that updated value.
func (s *startupDatabase) FlagsChanged(flagKeys ...keys.FlagKey) {
	if err := s.setFlags(context.Background()); err != nil {
		s.knapsack.Slogger().Log(context.Background(), slog.LevelError, "could not set flags after change", "err", err)
	}
}

func (s *startupDatabase) Close() error {
	return s.conn.Close()
}

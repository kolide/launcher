// Package startup provides access to and manages storage of startup data:
// flags/config values/settings that launcher needs during initialization,
// before the knapsack is available.
package startup

import (
	"context"
	"fmt"
	"log/slog"

	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	_ "modernc.org/sqlite"
)

// GetStartupValue retrieves the value for the given flagKey from the startup database
// located in the given rootDirectory. It wraps creation and closing of the sqlite store.
func GetStartupValue(ctx context.Context, rootDirectory string, flagKey string) (string, error) {
	store, err := agentsqlite.NewStore(ctx, rootDirectory, agentsqlite.TableStartupSettings)
	if err != nil {
		return "", fmt.Errorf("opening startup db in %s: %w", rootDirectory, err)
	}
	defer store.Close()

	flagValue, err := store.Get([]byte(flagKey))
	if err != nil {
		return "", fmt.Errorf("getting flag value %s: %w", flagKey, err)
	}

	return string(flagValue), nil
}

// startupDatabase records agent flags and their current values,
// responding to updates as a types.FlagsChangeObserver
type startupDatabase struct {
	kvStore     *agentsqlite.SqliteStore
	knapsack    types.Knapsack
	storedFlags map[keys.FlagKey]func() string // maps the agent flags to their knapsack getter functions
}

// NewStartupDatabase returns a new startup database, creating and initializing
// the database if necessary.
func NewStartupDatabase(ctx context.Context, knapsack types.Knapsack) (*startupDatabase, error) {
	store, err := agentsqlite.NewStore(ctx, knapsack.RootDirectory(), agentsqlite.TableStartupSettings)
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", knapsack.RootDirectory(), err)
	}

	s := &startupDatabase{
		kvStore:  store,
		knapsack: knapsack,
		storedFlags: map[keys.FlagKey]func() string{
			keys.UpdateChannel:     func() string { return knapsack.UpdateChannel() },
			keys.UseTUFAutoupdater: func() string { return flags.BoolToString(knapsack.UseTUFAutoupdater()) },
		},
	}

	// Attempt to ensure flags are up-to-date
	if err := s.setFlags(ctx); err != nil {
		s.knapsack.Slogger().Log(ctx, slog.LevelWarn, "could not set flags", "err", err)
	}

	for k := range s.storedFlags {
		s.knapsack.RegisterChangeObserver(s, k)
	}

	return s, nil
}

// setFlags updates the flags with their values from the agent flag data store.
func (s *startupDatabase) setFlags(ctx context.Context) error {
	updatedFlags := make(map[string]string)
	for flag, getter := range s.storedFlags {
		updatedFlags[flag.String()] = getter()
	}

	if _, err := s.kvStore.Update(updatedFlags); err != nil {
		return fmt.Errorf("updating flags: %w", err)
	}

	return nil
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface. When a flag
// that the startup database is registered for has a new value, the startup database
// stores that updated value.
func (s *startupDatabase) FlagsChanged(flagKeys ...keys.FlagKey) {
	if err := s.setFlags(context.Background()); err != nil {
		s.knapsack.Slogger().Log(context.Background(), slog.LevelError,
			"could not set flags after change",
			"err", err,
		)
	}
}

func (s *startupDatabase) Close() error {
	return s.kvStore.Close()
}

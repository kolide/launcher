// Package startupsettings provides access to and manages storage of startup data:
// flags/config values/settings that launcher needs during initialization,
// before the knapsack is available.
package startupsettings

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

// startupSettingsWriter records agent flags and their current values,
// responding to updates as a types.FlagsChangeObserver
type startupSettingsWriter struct {
	kvStore     types.GetterUpdaterCloser
	knapsack    types.Knapsack
	storedFlags map[keys.FlagKey]func() string // maps the agent flags to their knapsack getter functions
}

// OpenWriter returns a new startup settings writer, creating and initializing
// the database if necessary.
func OpenWriter(ctx context.Context, knapsack types.Knapsack) (*startupSettingsWriter, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	store, err := agentsqlite.OpenRW(ctx, knapsack.RootDirectory(), agentsqlite.StartupSettingsStore)
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", knapsack.RootDirectory(), err)
	}

	s := &startupSettingsWriter{
		kvStore:  store,
		knapsack: knapsack,
		storedFlags: map[keys.FlagKey]func() string{
			keys.UpdateChannel:         func() string { return knapsack.UpdateChannel() },
			keys.PinnedLauncherVersion: func() string { return knapsack.PinnedLauncherVersion() },
			keys.PinnedOsquerydVersion: func() string { return knapsack.PinnedOsquerydVersion() },
		},
	}

	// Attempt to ensure flags are up-to-date
	if err := s.setFlags(); err != nil {
		s.knapsack.Slogger().Log(ctx, slog.LevelWarn, "could not set flags", "err", err)
	}

	for k := range s.storedFlags {
		s.knapsack.RegisterChangeObserver(s, k)
	}

	return s, nil
}

// setFlags updates the flags with their values from the agent flag data store.
func (s *startupSettingsWriter) setFlags() error {
	updatedFlags := make(map[string]string)
	for flag, getter := range s.storedFlags {
		updatedFlags[flag.String()] = getter()
	}
	updatedFlags["use_tuf_autoupdater"] = "enabled" // Hardcode for backwards compatibility circa v1.5.3

	if _, err := s.kvStore.Update(updatedFlags); err != nil {
		return fmt.Errorf("updating flags: %w", err)
	}

	return nil
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface. When a flag
// that the startup database is registered for has a new value, the startup database
// stores that updated value.
func (s *startupSettingsWriter) FlagsChanged(flagKeys ...keys.FlagKey) {
	if err := s.setFlags(); err != nil {
		s.knapsack.Slogger().Log(context.Background(), slog.LevelError,
			"could not set flags after change",
			"err", err,
		)
	}
}

func (s *startupSettingsWriter) Close() error {
	return s.kvStore.Close()
}

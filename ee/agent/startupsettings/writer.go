// Package startupsettings provides access to and manages storage of startup data:
// flags/config values/settings that launcher needs during initialization,
// before the knapsack is available.
package startupsettings

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
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

	for k := range s.storedFlags {
		s.knapsack.RegisterChangeObserver(s, k)
	}

	return s, nil
}

// Ping satisfies the control.subscriber interface -- the runner subscribes to changes to
// the katc_config subsystem.
func (s *startupSettingsWriter) Ping() {
	if err := s.WriteSettings(); err != nil {
		s.knapsack.Slogger().Log(context.TODO(), slog.LevelWarn,
			"could not write updated settings",
			"err", err,
		)
	}
}

// WriteSettings updates the flags with their values from the agent flag data store.
func (s *startupSettingsWriter) WriteSettings() error {
	updatedFlags := make(map[string]string)
	for flag, getter := range s.storedFlags {
		updatedFlags[flag.String()] = getter()
	}
	updatedFlags["use_tuf_autoupdater"] = "enabled" // Hardcode for backwards compatibility circa v1.5.3

	for _, registrationId := range s.knapsack.RegistrationIDs() {
		atcConfig, err := s.extractAutoTableConstructionConfig(registrationId)
		if err != nil {
			s.knapsack.Slogger().Log(context.TODO(), slog.LevelDebug,
				"extracting auto_table_construction config",
				"err", err,
			)
		} else {
			atcConfigKey := storage.KeyByIdentifier([]byte("auto_table_construction"), storage.IdentifierTypeRegistration, []byte(registrationId))
			updatedFlags[string(atcConfigKey)] = atcConfig
		}

		if katcConfig, err := s.extractKATCConstructionConfig(registrationId); err != nil {
			s.knapsack.Slogger().Log(context.TODO(), slog.LevelDebug,
				"extracting katc_config",
				"err", err,
			)
		} else {
			katcConfigKey := storage.KeyByIdentifier([]byte("katc_config"), storage.IdentifierTypeRegistration, []byte(registrationId))
			updatedFlags[string(katcConfigKey)] = katcConfig
		}
	}

	if _, err := s.kvStore.Update(updatedFlags); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	return nil
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface. When a flag
// that the startup database is registered for has a new value, the startup database
// stores that updated value.
func (s *startupSettingsWriter) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	if err := s.WriteSettings(); err != nil {
		s.knapsack.Slogger().Log(ctx, slog.LevelError,
			"writing startup settings after flag change",
			"err", err,
		)
	}
}

func (s *startupSettingsWriter) Close() error {
	return s.kvStore.Close()
}

func (s *startupSettingsWriter) extractAutoTableConstructionConfig(registrationId string) (string, error) {
	osqConfig, err := s.knapsack.ConfigStore().Get(storage.KeyByIdentifier([]byte("config"), storage.IdentifierTypeRegistration, []byte(registrationId)))
	if err != nil {
		return "", fmt.Errorf("could not get osquery config from store: %w", err)
	}

	// convert osquery config to map
	var configUnmarshalled map[string]json.RawMessage
	if err := json.Unmarshal(osqConfig, &configUnmarshalled); err != nil {
		return "", fmt.Errorf("could not unmarshal osquery config: %w", err)
	}

	// delete what we don't want
	for k := range configUnmarshalled {
		if k == "auto_table_construction" {
			continue
		}
		delete(configUnmarshalled, k)
	}

	atcJson, err := json.Marshal(configUnmarshalled)
	if err != nil {
		return "", fmt.Errorf("could not marshal auto_table_construction: %w", err)
	}

	return string(atcJson), nil
}

func (s *startupSettingsWriter) extractKATCConstructionConfig(registrationId string) (string, error) {
	kolideCfg := make(map[string]string)
	if err := s.knapsack.KatcConfigStore().ForEach(func(k []byte, v []byte) error {
		key, _, identifier := storage.SplitKey(k)
		if string(identifier) == registrationId {
			kolideCfg[string(key)] = string(v)
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("could not get Kolide ATC config from store: %w", err)
	}

	atcJson, err := json.Marshal(kolideCfg)
	if err != nil {
		return "", fmt.Errorf("could not marshal katc_config: %w", err)
	}

	return string(atcJson), nil
}

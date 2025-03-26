package table

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/startupsettings"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/katc"
	"github.com/kolide/launcher/ee/tables/cryptoinfotable"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/desktopprocs"
	"github.com/kolide/launcher/ee/tables/dev_table_tooling"
	"github.com/kolide/launcher/ee/tables/firefox_preferences"
	"github.com/kolide/launcher/ee/tables/jwt"
	"github.com/kolide/launcher/ee/tables/launcher_db"
	"github.com/kolide/launcher/ee/tables/osquery_instance_history"
	"github.com/kolide/launcher/ee/tables/tdebug"
	"github.com/kolide/launcher/ee/tables/tufinfo"

	osquery "github.com/osquery/osquery-go"
)

// LauncherTables returns launcher-specific tables. They're based
// around _launcher_ things thus do not make sense in tables.ext
func LauncherTables(k types.Knapsack, slogger *slog.Logger) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		LauncherConfigTable(k, slogger, k.ConfigStore(), k),
		LauncherDbInfo(k, slogger, k.BboltDB()),
		LauncherInfoTable(k, slogger, k.ConfigStore(), k.LauncherHistoryStore()),
		launcher_db.TablePlugin(k, slogger, "kolide_server_data", k.ServerProvidedDataStore()),
		launcher_db.TablePlugin(k, slogger, "kolide_control_flags", k.AgentFlagsStore()),
		LauncherAutoupdateConfigTable(slogger, k),
		osquery_instance_history.TablePlugin(k, slogger),
		tufinfo.TufReleaseVersionTable(slogger, k),
		launcher_db.TablePlugin(k, slogger, "kolide_tuf_autoupdater_errors", k.AutoupdateErrorsStore()),
		desktopprocs.TablePlugin(k, slogger),
	}
}

// PlatformTables returns all tables for the launcher build platform.
func PlatformTables(k types.Knapsack, registrationId string, slogger *slog.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	// Common tables to all platforms
	tables := []osquery.OsqueryPlugin{
		ChromeLoginDataEmails(k, slogger),
		ChromeUserProfiles(k, slogger),
		KeyInfo(k, slogger),
		OnePasswordAccounts(k, slogger),
		SlackConfig(k, slogger),
		SshKeys(k, slogger),
		cryptoinfotable.TablePlugin(k, slogger),
		dev_table_tooling.TablePlugin(k, slogger),
		firefox_preferences.TablePlugin(k, slogger),
		jwt.TablePlugin(k, slogger),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_zerotier_info", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"info"}),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_zerotier_networks", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"listnetworks"}),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_zerotier_peers", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"listpeers"}),
		tdebug.LauncherGcInfo(k, slogger),
	}

	// The dataflatten tables
	tables = append(tables, dataflattentable.AllTablePlugins(k, slogger)...)

	// add in the platform specific ones (as denoted by build tags)
	tables = append(tables, platformSpecificTables(k, slogger, currentOsquerydBinaryPath)...)

	return tables
}

// KolideCustomAtcTables retrieves Kolide ATC config from the appropriate data store(s),
// then constructs the tables.
func KolideCustomAtcTables(k types.Knapsack, registrationId string, slogger *slog.Logger) []osquery.OsqueryPlugin {
	// Fetch tables from KVStore or from startup settings
	config, err := katcFromDb(k, registrationId)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelDebug,
			"could not retrieve KATC config from store, may not have access -- falling back to startup settings",
			"err", err,
		)

		config, err = katcFromStartupSettings(k, registrationId)
		if err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"could not retrieve KATC config from startup settings",
				"err", err,
			)
			return nil
		}
	}

	return katc.ConstructKATCTables(config, k, slogger)
}

func katcFromDb(k types.Knapsack, registrationId string) (map[string]string, error) {
	if k == nil || k.KatcConfigStore() == nil {
		return nil, errors.New("stores in knapsack not available")
	}
	katcCfg := make(map[string]string)
	if err := k.KatcConfigStore().ForEach(func(k []byte, v []byte) error {
		key, _, identifier := storage.SplitKey(k)
		if string(identifier) == registrationId {
			katcCfg[string(key)] = string(v)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("retrieving contents of Kolide ATC config store: %w", err)
	}

	return katcCfg, nil
}

func katcFromStartupSettings(k types.Knapsack, registrationId string) (map[string]string, error) {
	r, err := startupsettings.OpenReader(context.TODO(), k.Slogger(), k.RootDirectory())
	if err != nil {
		return nil, fmt.Errorf("error opening startup settings reader: %w", err)
	}
	defer r.Close()

	katcConfigKey := storage.KeyByIdentifier([]byte("katc_config"), storage.IdentifierTypeRegistration, []byte(registrationId))
	katcConfig, err := r.Get(string(katcConfigKey))
	if err != nil {
		return nil, fmt.Errorf("error getting katc_config from startup settings: %w", err)
	}

	var katcCfg map[string]string
	if err := json.Unmarshal([]byte(katcConfig), &katcCfg); err != nil {
		return nil, fmt.Errorf("unmarshalling katc_config: %w", err)
	}

	return katcCfg, nil
}

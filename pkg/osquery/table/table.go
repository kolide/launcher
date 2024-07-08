package table

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/startupsettings"
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
func LauncherTables(k types.Knapsack) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		LauncherConfigTable(k.ConfigStore()),
		LauncherDbInfo(k.BboltDB()),
		LauncherInfoTable(k.ConfigStore()),
		launcher_db.TablePlugin("kolide_server_data", k.ServerProvidedDataStore()),
		launcher_db.TablePlugin("kolide_control_flags", k.AgentFlagsStore()),
		LauncherAutoupdateConfigTable(k),
		osquery_instance_history.TablePlugin(),
		tufinfo.TufReleaseVersionTable(k),
		launcher_db.TablePlugin("kolide_tuf_autoupdater_errors", k.AutoupdateErrorsStore()),
		desktopprocs.TablePlugin(),
	}
}

// PlatformTables returns all tables for the launcher build platform.
func PlatformTables(k types.Knapsack, slogger *slog.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	// Common tables to all platforms
	tables := []osquery.OsqueryPlugin{
		ChromeLoginDataEmails(slogger),
		ChromeUserProfiles(slogger),
		KeyInfo(slogger),
		OnePasswordAccounts(slogger),
		SlackConfig(slogger),
		SshKeys(slogger),
		cryptoinfotable.TablePlugin(slogger),
		dev_table_tooling.TablePlugin(slogger),
		firefox_preferences.TablePlugin(slogger),
		jwt.TablePlugin(slogger),
		dataflattentable.TablePluginExec(slogger,
			"kolide_zerotier_info", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"info"}),
		dataflattentable.TablePluginExec(slogger,
			"kolide_zerotier_networks", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"listnetworks"}),
		dataflattentable.TablePluginExec(slogger,
			"kolide_zerotier_peers", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"listpeers"}),
		tdebug.LauncherGcInfo(slogger),
	}

	// The dataflatten tables
	tables = append(tables, dataflattentable.AllTablePlugins(slogger)...)

	// add in the platform specific ones (as denoted by build tags)
	tables = append(tables, platformSpecificTables(slogger, currentOsquerydBinaryPath)...)

	// Add in the Kolide custom ATC tables
	tables = append(tables, kolideCustomAtcTables(k, slogger)...)

	return tables
}

// kolideCustomAtcTables retrieves Kolide ATC config from the appropriate data store(s).
// For now, it just logs the configuration. In the future, it will handle indexeddb tables
// and others.
func kolideCustomAtcTables(k types.Knapsack, slogger *slog.Logger) []osquery.OsqueryPlugin {
	// Fetch tables from KVStore or from startup settings
	config, err := katcFromDb(k)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelDebug,
			"could not retrieve Kolide ATC config from store, may not have access -- falling back to startup settings",
			"err", err,
		)

		config, err = katcFromStartupSettings(k)
		if err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"could not retrieve Kolide ATC config from startup settings",
				"err", err,
			)
			return nil
		}
	}

	return katc.ConstructKATCTables(config, slogger)
}

func katcFromDb(k types.Knapsack) (map[string]string, error) {
	if k == nil || k.KatcConfigStore() == nil {
		return nil, errors.New("stores in knapsack not available")
	}
	katcCfg := make(map[string]string)
	if err := k.KatcConfigStore().ForEach(func(k []byte, v []byte) error {
		katcCfg[string(k)] = string(v)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("retrieving contents of Kolide ATC config store: %w", err)
	}

	return katcCfg, nil
}

func katcFromStartupSettings(k types.Knapsack) (map[string]string, error) {
	r, err := startupsettings.OpenReader(context.TODO(), k.RootDirectory())
	if err != nil {
		return nil, fmt.Errorf("error opening startup settings reader: %w", err)
	}
	defer r.Close()

	katcConfig, err := r.Get("katc_config")
	if err != nil {
		return nil, fmt.Errorf("error getting katc_config from startup settings: %w", err)
	}

	var katcCfg map[string]string
	if err := json.Unmarshal([]byte(katcConfig), &katcCfg); err != nil {
		return nil, fmt.Errorf("unmarshalling katc_config: %w", err)
	}

	return katcCfg, nil
}

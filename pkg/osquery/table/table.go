package table

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
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

// kolideCustomAtcTables will handle indexeddb tables and others in the future. For now,
// it just logs the config.
func kolideCustomAtcTables(k types.Knapsack, slogger *slog.Logger) []osquery.OsqueryPlugin {
	loggableConfig := make(map[string]string)
	if err := k.AtcConfigStore().ForEach(func(k []byte, v []byte) error {
		loggableConfig[string(k)] = string(v)
		return nil
	}); err != nil {
		slogger.Log(context.TODO(), slog.LevelDebug,
			"could not retrieve contents of Kolide ATC config store",
			"err", err,
		)
		return nil
	}

	slogger.Log(context.TODO(), slog.LevelDebug,
		"retrieved contents of Kolide ATC config store",
		"config", loggableConfig,
	)
	return nil
}

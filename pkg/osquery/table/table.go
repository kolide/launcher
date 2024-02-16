package table

import (
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/cryptoinfotable"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/desktopprocs"
	"github.com/kolide/launcher/ee/tables/dev_table_tooling"
	"github.com/kolide/launcher/ee/tables/firefox_preferences"
	"github.com/kolide/launcher/ee/tables/launcher_db"
	"github.com/kolide/launcher/ee/tables/osquery_instance_history"
	"github.com/kolide/launcher/ee/tables/tdebug"
	"github.com/kolide/launcher/ee/tables/tufinfo"

	"github.com/go-kit/kit/log"
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
func PlatformTables(slogger *slog.Logger, logger log.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	// Common tables to all platforms
	tables := []osquery.OsqueryPlugin{
		ChromeLoginDataEmails(slogger),
		ChromeUserProfiles(slogger),
		KeyInfo(slogger),
		OnePasswordAccounts(slogger),
		SlackConfig(slogger),
		SshKeys(slogger),
		cryptoinfotable.TablePlugin(slogger, logger),
		dev_table_tooling.TablePlugin(slogger, logger),
		firefox_preferences.TablePlugin(slogger),
		dataflattentable.TablePluginExec(logger,
			"kolide_zerotier_info", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"info"}),
		dataflattentable.TablePluginExec(logger,
			"kolide_zerotier_networks", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"listnetworks"}),
		dataflattentable.TablePluginExec(logger,
			"kolide_zerotier_peers", dataflattentable.JsonType, allowedcmd.ZerotierCli, []string{"listpeers"}),
		tdebug.LauncherGcInfo(slogger, logger),
	}

	// The dataflatten tables
	tables = append(tables, dataflattentable.AllTablePlugins(logger)...)

	// add in the platform specific ones (as denoted by build tags)
	tables = append(tables, platformSpecificTables(slogger, logger, currentOsquerydBinaryPath)...)

	return tables
}

package table

import (
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/osquery/tables/cryptoinfotable"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/dev_table_tooling"
	"github.com/kolide/launcher/pkg/osquery/tables/firefox_preferences"
	"github.com/kolide/launcher/pkg/osquery/tables/osquery_instance_history"
	"github.com/kolide/launcher/pkg/osquery/tables/tdebug"
	"github.com/kolide/launcher/pkg/osquery/tables/zfs"

	"github.com/go-kit/kit/log"
	osquery "github.com/osquery/osquery-go"
	"go.etcd.io/bbolt"
)

// LauncherTables returns launcher-specific tables. They're based
// around _launcher_ things thus do not make sense in tables.ext
func LauncherTables(db *bbolt.DB, opts *launcher.Options) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		LauncherConfigTable(db),
		LauncherDbInfo(db),
		LauncherInfoTable(db),
		TargetMembershipTable(db),
		LauncherAutoupdateConfigTable(opts),
		osquery_instance_history.TablePlugin(),
	}
}

// PlatformTables returns all tables for the launcher build platform.
func PlatformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	// Common tables to all platforms
	tables := []osquery.OsqueryPlugin{
		BestPractices(client),
		ChromeLoginDataEmails(client, logger),
		ChromeUserProfiles(client, logger),
		EmailAddresses(client, logger),
		KeyInfo(client, logger),
		OnePasswordAccounts(client, logger),
		SlackConfig(client, logger),
		SshKeys(client, logger),
		cryptoinfotable.TablePlugin(logger),
		dev_table_tooling.TablePlugin(client, logger),
		firefox_preferences.TablePlugin(logger),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_zerotier_info", dataflattentable.JsonType, zerotierCli("info")),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_zerotier_networks", dataflattentable.JsonType, zerotierCli("listnetworks")),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_zerotier_peers", dataflattentable.JsonType, zerotierCli("listpeers")),
		tdebug.LauncherGcInfo(client, logger),
		zfs.ZfsPropertiesPlugin(client, logger),
		zfs.ZpoolPropertiesPlugin(client, logger),
	}

	// The dataflatten tables
	tables = append(tables, dataflattentable.AllTablePlugins(client, logger)...)

	// add in the platform specific ones (as denoted by build tags)
	tables = append(tables, platformTables(client, logger, currentOsquerydBinaryPath)...)

	return tables
}

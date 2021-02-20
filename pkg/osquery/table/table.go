package table

import (
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/zfs"

	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"go.etcd.io/bbolt"
)

// LauncherTables returns launcher-specific tables
func LauncherTables(db *bbolt.DB, opts *launcher.Options) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		LauncherConfigTable(db),
		LauncherIdentifierTable(db),
		TargetMembershipTable(db),
		LauncherAutoupdateConfigTable(opts),
	}
}

// PlatformTables returns all tables for the launcher build platform.
func PlatformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	// Common tables to all platforms
	tables := []*table.Plugin{
		BestPractices(client),
		ChromeLoginDataEmails(client, logger),
		ChromeUserProfiles(client, logger),
		EmailAddresses(client, logger),
		KeyInfo(client, logger),
		LauncherInfoTable(),
		OnePasswordAccounts(client, logger),
		SlackConfig(client, logger),
		SshKeys(client, logger),
		dataflattentable.TablePlugin(client, logger, dataflattentable.JsonType),
		dataflattentable.TablePlugin(client, logger, dataflattentable.XmlType),
		dataflattentable.TablePlugin(client, logger, dataflattentable.IniType),
		dataflattentable.TablePlugin(client, logger, dataflattentable.PlistType),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_zerotier_info", dataflattentable.JsonType, zerotierCli("info")),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_zerotier_networks", dataflattentable.JsonType, zerotierCli("listnetworks")),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_zerotier_peers", dataflattentable.JsonType, zerotierCli("listpeers")),
		zfs.ZfsPropertiesPlugin(client, logger),
		zfs.ZpoolPropertiesPlugin(client, logger),
	}

	// add in the platform specific ones (as denoted by build tags)
	tables = append(tables, platformTables(client, logger, currentOsquerydBinaryPath)...)

	return tables
}

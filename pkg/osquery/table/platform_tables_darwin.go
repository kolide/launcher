// +build darwin

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/knightsc/system_policy/osquery/table/kextpolicy"
	"github.com/knightsc/system_policy/osquery/table/legacyexec"
	appicons "github.com/kolide/launcher/pkg/osquery/tables/app-icons"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/filevault"
	"github.com/kolide/launcher/pkg/osquery/tables/firmwarepasswd"
	"github.com/kolide/launcher/pkg/osquery/tables/ioreg"
	"github.com/kolide/launcher/pkg/osquery/tables/mdmclient"
	"github.com/kolide/launcher/pkg/osquery/tables/munki"
	"github.com/kolide/launcher/pkg/osquery/tables/osquery_user_exec_table"
	"github.com/kolide/launcher/pkg/osquery/tables/profiles"
	"github.com/kolide/launcher/pkg/osquery/tables/pwpolicy"
	"github.com/kolide/launcher/pkg/osquery/tables/systemprofiler"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	_ "github.com/mattn/go-sqlite3"
)

const (
	keychainAclsQuery  = "select * from keychain_acls"
	keychainItemsQuery = "select * from keychain_items"
	screenlockQuery    = "select enabled, grace_period from screenlock"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	munki := munki.New()

	// This table uses undocumented APIs, There is some discussion at the
	// PR adding the table. See
	// https://github.com/osquery/osquery/pull/6243
	screenlockTable := osquery_user_exec_table.TablePlugin(
		client, logger, "kolide_screenlock",
		currentOsquerydBinaryPath, screenlockQuery,
		[]table.ColumnDefinition{
			table.IntegerColumn("enabled"),
			table.IntegerColumn("grace_period"),
		})

	keychainAclsTable := osquery_user_exec_table.TablePlugin(
		client, logger, "kolide_keychain_acls",
		currentOsquerydBinaryPath, keychainItemsQuery,
		[]table.ColumnDefinition{
			table.TextColumn("keychain_path"),
			table.TextColumn("authorizations"),
			table.TextColumn("path"),
			table.TextColumn("description"),
			table.TextColumn("label"),
		})

	keychainItemsTable := osquery_user_exec_table.TablePlugin(
		client, logger, "kolide_keychain_items",
		currentOsquerydBinaryPath, keychainAclsQuery,
		[]table.ColumnDefinition{
			table.TextColumn("label"),
			table.TextColumn("description"),
			table.TextColumn("comment"),
			table.TextColumn("created"),
			table.TextColumn("modified"),
			table.TextColumn("type"),
			table.TextColumn("path"),
		})

	return []*table.Plugin{
		keychainAclsTable,
		keychainItemsTable,
		Airdrop(client),
		appicons.AppIcons(),
		ChromeLoginKeychainInfo(client, logger),
		firmwarepasswd.TablePlugin(client, logger),
		GDriveSyncConfig(client, logger),
		GDriveSyncHistoryInfo(client, logger),
		KolideVulnerabilities(client, logger),
		MDMInfo(logger),
		MacOSUpdate(client),
		MachoInfo(),
		Spotlight(),
		TouchIDUserConfig(client, logger),
		TouchIDSystemConfig(client, logger),
		UserAvatar(logger),
		ioreg.TablePlugin(client, logger),
		profiles.TablePlugin(client, logger),
		kextpolicy.TablePlugin(),
		filevault.TablePlugin(client, logger),
		mdmclient.TablePlugin(client, logger),
		legacyexec.TablePlugin(),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_diskutil_list", dataflattentable.PlistType, []string{"/usr/sbin/diskutil", "list", "-plist"}),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_apfs_list", dataflattentable.PlistType, []string{"/usr/sbin/diskutil", "apfs", "list", "-plist"}),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_apfs_users", dataflattentable.PlistType, []string{"/usr/sbin/diskutil", "apfs", "listUsers", "/", "-plist"}),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_tmutil_destinationinfo", dataflattentable.PlistType, []string{"/usr/bin/tmutil", "destinationinfo", "-X"}),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_powermetrics", dataflattentable.PlistType, []string{"/usr/bin/powermetrics", "-n", "1", "-f", "plist"}),
		screenlockTable,
		pwpolicy.TablePlugin(client, logger),
		systemprofiler.TablePlugin(client, logger),
		munki.ManagedInstalls(client, logger),
		munki.MunkiReport(client, logger),
	}
}

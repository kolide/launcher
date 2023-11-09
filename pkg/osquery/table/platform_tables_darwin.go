//go:build darwin
// +build darwin

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/knightsc/system_policy/osquery/table/kextpolicy"
	"github.com/knightsc/system_policy/osquery/table/legacyexec"
	"github.com/kolide/launcher/pkg/allowedpaths"
	"github.com/kolide/launcher/pkg/osquery/tables/airport"
	appicons "github.com/kolide/launcher/pkg/osquery/tables/app-icons"
	"github.com/kolide/launcher/pkg/osquery/tables/apple_silicon_security_policy"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/remotectl"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/repcli"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/softwareupdate"
	"github.com/kolide/launcher/pkg/osquery/tables/filevault"
	"github.com/kolide/launcher/pkg/osquery/tables/firmwarepasswd"
	"github.com/kolide/launcher/pkg/osquery/tables/ioreg"
	"github.com/kolide/launcher/pkg/osquery/tables/macos_software_update"
	"github.com/kolide/launcher/pkg/osquery/tables/mdmclient"
	"github.com/kolide/launcher/pkg/osquery/tables/munki"
	"github.com/kolide/launcher/pkg/osquery/tables/osquery_user_exec_table"
	"github.com/kolide/launcher/pkg/osquery/tables/profiles"
	"github.com/kolide/launcher/pkg/osquery/tables/pwpolicy"
	"github.com/kolide/launcher/pkg/osquery/tables/systemprofiler"
	"github.com/kolide/launcher/pkg/osquery/tables/zfs"
	_ "github.com/mattn/go-sqlite3"
	osquery "github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	keychainAclsQuery  = "select * from keychain_acls"
	keychainItemsQuery = "select * from keychain_items"
	screenlockQuery    = "select enabled, grace_period from screenlock"
)

func platformTables(logger log.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	munki := munki.New()

	// This table uses undocumented APIs, There is some discussion at the
	// PR adding the table. See
	// https://github.com/osquery/osquery/pull/6243
	screenlockTable := osquery_user_exec_table.TablePlugin(
		logger, "kolide_screenlock",
		currentOsquerydBinaryPath, screenlockQuery,
		[]table.ColumnDefinition{
			table.IntegerColumn("enabled"),
			table.IntegerColumn("grace_period"),
		})

	keychainAclsTable := osquery_user_exec_table.TablePlugin(
		logger, "kolide_keychain_acls",
		currentOsquerydBinaryPath, keychainItemsQuery,
		[]table.ColumnDefinition{
			table.TextColumn("keychain_path"),
			table.TextColumn("authorizations"),
			table.TextColumn("path"),
			table.TextColumn("description"),
			table.TextColumn("label"),
		})

	keychainItemsTable := osquery_user_exec_table.TablePlugin(
		logger, "kolide_keychain_items",
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

	return []osquery.OsqueryPlugin{
		keychainAclsTable,
		keychainItemsTable,
		appicons.AppIcons(),
		ChromeLoginKeychainInfo(logger),
		firmwarepasswd.TablePlugin(logger),
		GDriveSyncConfig(logger),
		GDriveSyncHistoryInfo(logger),
		MDMInfo(logger),
		macos_software_update.MacOSUpdate(),
		macos_software_update.RecommendedUpdates(logger),
		macos_software_update.AvailableProducts(logger),
		MachoInfo(),
		Spotlight(),
		TouchIDUserConfig(logger),
		TouchIDSystemConfig(logger),
		UserAvatar(logger),
		ioreg.TablePlugin(logger),
		profiles.TablePlugin(logger),
		airport.TablePlugin(logger),
		kextpolicy.TablePlugin(),
		filevault.TablePlugin(logger),
		mdmclient.TablePlugin(logger),
		apple_silicon_security_policy.TablePlugin(logger),
		legacyexec.TablePlugin(),
		dataflattentable.TablePluginExec(logger,
			"kolide_diskutil_list", dataflattentable.PlistType, allowedpaths.Diskutil, []string{"list", "-plist"}),
		dataflattentable.TablePluginExec(logger,
			"kolide_falconctl_stats", dataflattentable.PlistType, allowedpaths.Falconctl, []string{"stats", "-p"}),
		dataflattentable.TablePluginExec(logger,
			"kolide_apfs_list", dataflattentable.PlistType, allowedpaths.Diskutil, []string{"apfs", "list", "-plist"}),
		dataflattentable.TablePluginExec(logger,
			"kolide_apfs_users", dataflattentable.PlistType, allowedpaths.Diskutil, []string{"apfs", "listUsers", "/", "-plist"}),
		dataflattentable.TablePluginExec(logger,
			"kolide_tmutil_destinationinfo", dataflattentable.PlistType, allowedpaths.Tmutil, []string{"destinationinfo", "-X"}),
		dataflattentable.TablePluginExec(logger,
			"kolide_powermetrics", dataflattentable.PlistType, allowedpaths.Powermetrics, []string{"-n", "1", "-f", "plist"}),
		screenlockTable,
		pwpolicy.TablePlugin(logger),
		systemprofiler.TablePlugin(logger),
		munki.ManagedInstalls(logger),
		munki.MunkiReport(logger),
		dataflattentable.NewExecAndParseTable(logger, "kolide_remotectl", remotectl.Parser, allowedpaths.Remotectl, []string{`dumpstate`}),
		dataflattentable.NewExecAndParseTable(logger, "kolide_softwareupdate", softwareupdate.Parser, allowedpaths.Softwareupdate, []string{`--list`, `--no-scan`}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_softwareupdate_scan", softwareupdate.Parser, allowedpaths.Softwareupdate, []string{`--list`}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_carbonblack_repcli_status", repcli.Parser, allowedpaths.Repcli, []string{"status"}, dataflattentable.WithIncludeStderr()),
		zfs.ZfsPropertiesPlugin(logger),
		zfs.ZpoolPropertiesPlugin(logger),
	}
}

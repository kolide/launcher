//go:build darwin
// +build darwin

package table

import (
	"log/slog"

	"github.com/knightsc/system_policy/osquery/table/kextpolicy"
	"github.com/knightsc/system_policy/osquery/table/legacyexec"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/airport"
	appicons "github.com/kolide/launcher/ee/tables/app-icons"
	"github.com/kolide/launcher/ee/tables/apple_silicon_security_policy"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/execparsers/remotectl"
	"github.com/kolide/launcher/ee/tables/execparsers/repcli"
	"github.com/kolide/launcher/ee/tables/execparsers/socketfilterfw"
	"github.com/kolide/launcher/ee/tables/execparsers/softwareupdate"
	"github.com/kolide/launcher/ee/tables/filevault"
	"github.com/kolide/launcher/ee/tables/firmwarepasswd"
	brew_upgradeable "github.com/kolide/launcher/ee/tables/homebrew"
	"github.com/kolide/launcher/ee/tables/ioreg"
	"github.com/kolide/launcher/ee/tables/macos_software_update"
	"github.com/kolide/launcher/ee/tables/mdmclient"
	"github.com/kolide/launcher/ee/tables/munki"
	"github.com/kolide/launcher/ee/tables/osquery_user_exec_table"
	"github.com/kolide/launcher/ee/tables/profiles"
	"github.com/kolide/launcher/ee/tables/pwpolicy"
	"github.com/kolide/launcher/ee/tables/spotlight"
	"github.com/kolide/launcher/ee/tables/systemprofiler"
	"github.com/kolide/launcher/ee/tables/zfs"
	osquery "github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	keychainAclsQuery  = "select * from keychain_acls"
	keychainItemsQuery = "select * from keychain_items"
	screenlockQuery    = "select enabled, grace_period from screenlock"
)

func platformSpecificTables(k types.Knapsack, slogger *slog.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	munki := munki.New()

	// This table uses undocumented APIs, There is some discussion at the
	// PR adding the table. See
	// https://github.com/osquery/osquery/pull/6243
	screenlockTable := osquery_user_exec_table.TablePlugin(
		k, slogger, "kolide_screenlock",
		currentOsquerydBinaryPath, screenlockQuery,
		[]table.ColumnDefinition{
			table.IntegerColumn("enabled"),
			table.IntegerColumn("grace_period"),
		})

	keychainAclsTable := osquery_user_exec_table.TablePlugin(
		k, slogger, "kolide_keychain_acls",
		currentOsquerydBinaryPath, keychainItemsQuery,
		[]table.ColumnDefinition{
			table.TextColumn("keychain_path"),
			table.TextColumn("authorizations"),
			table.TextColumn("path"),
			table.TextColumn("description"),
			table.TextColumn("label"),
		})

	keychainItemsTable := osquery_user_exec_table.TablePlugin(
		k, slogger, "kolide_keychain_items",
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
		appicons.AppIcons(k, slogger),
		brew_upgradeable.TablePlugin(k, slogger),
		ChromeLoginKeychainInfo(k, slogger),
		firmwarepasswd.TablePlugin(k, slogger),
		GDriveSyncConfig(k, slogger),
		GDriveSyncHistoryInfo(k, slogger),
		MDMInfo(k, slogger),
		macos_software_update.MacOSUpdate(k, slogger),
		macos_software_update.RecommendedUpdates(k, slogger),
		macos_software_update.AvailableProducts(k, slogger),
		MachoInfo(k, slogger),
		spotlight.TablePlugin(k, slogger),
		TouchIDUserConfig(k, slogger),
		TouchIDSystemConfig(k, slogger),
		UserAvatar(k, slogger),
		ioreg.TablePlugin(k, slogger),
		profiles.TablePlugin(k, slogger),
		airport.TablePlugin(k, slogger),
		kextpolicy.TablePlugin(),
		filevault.TablePlugin(k, slogger),
		mdmclient.TablePlugin(k, slogger),
		apple_silicon_security_policy.TablePlugin(k, slogger),
		legacyexec.TablePlugin(),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_diskutil_list", dataflattentable.PlistType, allowedcmd.Diskutil, []string{"list", "-plist"}),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_falconctl_stats", dataflattentable.PlistType, allowedcmd.Launcher, []string{"rundisclaimed", "falconctl", "stats", "-p"}),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_apfs_list", dataflattentable.PlistType, allowedcmd.Diskutil, []string{"apfs", "list", "-plist"}),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_apfs_users", dataflattentable.PlistType, allowedcmd.Diskutil, []string{"apfs", "listUsers", "/", "-plist"}),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_tmutil_destinationinfo", dataflattentable.PlistType, allowedcmd.Tmutil, []string{"destinationinfo", "-X"}),
		dataflattentable.TablePluginExec(k, slogger,
			"kolide_powermetrics", dataflattentable.PlistType, allowedcmd.Powermetrics, []string{"-n", "1", "-f", "plist"}),
		screenlockTable,
		pwpolicy.TablePlugin(k, slogger),
		systemprofiler.TablePlugin(k, slogger),
		munki.ManagedInstalls(k, slogger),
		munki.MunkiReport(k, slogger),
		dataflattentable.TablePluginExec(k, slogger, "kolide_nix_upgradeable", dataflattentable.XmlType, allowedcmd.NixEnv, []string{"--query", "--installed", "-c", "--xml"}),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_remotectl", remotectl.Parser, allowedcmd.Remotectl, []string{`dumpstate`}),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_socketfilterfw", socketfilterfw.Parser, allowedcmd.Socketfilterfw, []string{"--getglobalstate", "--getblockall", "--getallowsigned", "--getstealthmode"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_socketfilterfw_apps", socketfilterfw.Parser, allowedcmd.Socketfilterfw, []string{"--listapps"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_softwareupdate", softwareupdate.Parser, allowedcmd.Softwareupdate, []string{`--list`, `--no-scan`}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_softwareupdate_scan", softwareupdate.Parser, allowedcmd.Softwareupdate, []string{`--list`}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_carbonblack_repcli_status", repcli.Parser, allowedcmd.Launcher, []string{"rundisclaimed", "carbonblack_repcli", "status"}),
		zfs.ZfsPropertiesPlugin(k, slogger),
		zfs.ZpoolPropertiesPlugin(k, slogger),
	}
}

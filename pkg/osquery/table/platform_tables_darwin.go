//go:build darwin
// +build darwin

package table

import (
	"log/slog"

	"github.com/knightsc/system_policy/osquery/table/kextpolicy"
	"github.com/knightsc/system_policy/osquery/table/legacyexec"
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
	_ "github.com/mattn/go-sqlite3"
	osquery "github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	keychainAclsQuery  = "select * from keychain_acls"
	keychainItemsQuery = "select * from keychain_items"
	screenlockQuery    = "select enabled, grace_period from screenlock"
)

func platformSpecificTables(slogger *slog.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	munki := munki.New()

	// This table uses undocumented APIs, There is some discussion at the
	// PR adding the table. See
	// https://github.com/osquery/osquery/pull/6243
	screenlockTable := osquery_user_exec_table.TablePlugin(
		slogger, "kolide_screenlock",
		currentOsquerydBinaryPath, screenlockQuery,
		[]table.ColumnDefinition{
			table.IntegerColumn("enabled"),
			table.IntegerColumn("grace_period"),
		})

	keychainAclsTable := osquery_user_exec_table.TablePlugin(
		slogger, "kolide_keychain_acls",
		currentOsquerydBinaryPath, keychainItemsQuery,
		[]table.ColumnDefinition{
			table.TextColumn("keychain_path"),
			table.TextColumn("authorizations"),
			table.TextColumn("path"),
			table.TextColumn("description"),
			table.TextColumn("label"),
		})

	keychainItemsTable := osquery_user_exec_table.TablePlugin(
		slogger, "kolide_keychain_items",
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
		appicons.AppIcons(slogger),
		brew_upgradeable.TablePlugin(slogger),
		ChromeLoginKeychainInfo(slogger),
		firmwarepasswd.TablePlugin(slogger),
		GDriveSyncConfig(slogger),
		GDriveSyncHistoryInfo(slogger),
		MDMInfo(slogger),
		macos_software_update.MacOSUpdate(slogger),
		macos_software_update.RecommendedUpdates(slogger),
		macos_software_update.AvailableProducts(slogger),
		MachoInfo(slogger),
		spotlight.TablePlugin(slogger),
		TouchIDUserConfig(slogger),
		TouchIDSystemConfig(slogger),
		UserAvatar(slogger),
		ioreg.TablePlugin(slogger),
		profiles.TablePlugin(slogger),
		airport.TablePlugin(slogger),
		kextpolicy.TablePlugin(),
		filevault.TablePlugin(slogger),
		mdmclient.TablePlugin(slogger),
		apple_silicon_security_policy.TablePlugin(slogger),
		legacyexec.TablePlugin(),
		dataflattentable.TablePluginExec(slogger,
			"kolide_diskutil_list", dataflattentable.PlistType, allowedcmd.Diskutil, []string{"list", "-plist"}),
		dataflattentable.TablePluginExec(slogger,
			"kolide_falconctl_stats", dataflattentable.PlistType, allowedcmd.Falconctl, []string{"stats", "-p"}),
		dataflattentable.TablePluginExec(slogger,
			"kolide_apfs_list", dataflattentable.PlistType, allowedcmd.Diskutil, []string{"apfs", "list", "-plist"}),
		dataflattentable.TablePluginExec(slogger,
			"kolide_apfs_users", dataflattentable.PlistType, allowedcmd.Diskutil, []string{"apfs", "listUsers", "/", "-plist"}),
		dataflattentable.TablePluginExec(slogger,
			"kolide_tmutil_destinationinfo", dataflattentable.PlistType, allowedcmd.Tmutil, []string{"destinationinfo", "-X"}),
		dataflattentable.TablePluginExec(slogger,
			"kolide_powermetrics", dataflattentable.PlistType, allowedcmd.Powermetrics, []string{"-n", "1", "-f", "plist"}),
		screenlockTable,
		pwpolicy.TablePlugin(slogger),
		systemprofiler.TablePlugin(slogger),
		munki.ManagedInstalls(slogger),
		munki.MunkiReport(slogger),
		dataflattentable.TablePluginExec(slogger, "kolide_nix_upgradeable", dataflattentable.XmlType, allowedcmd.NixEnv, []string{"--query", "--installed", "-c", "--xml"}),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_remotectl", remotectl.Parser, allowedcmd.Remotectl, []string{`dumpstate`}),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_socketfilterfw", socketfilterfw.Parser, allowedcmd.Socketfilterfw, []string{"--getglobalstate", "--getblockall", "--getallowsigned", "--getstealthmode", "--getloggingmode", "--getloggingopt"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_socketfilterfw_apps", socketfilterfw.Parser, allowedcmd.Socketfilterfw, []string{"--listapps"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_softwareupdate", softwareupdate.Parser, allowedcmd.Softwareupdate, []string{`--list`, `--no-scan`}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_softwareupdate_scan", softwareupdate.Parser, allowedcmd.Softwareupdate, []string{`--list`}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_carbonblack_repcli_status", repcli.Parser, allowedcmd.Repcli, []string{"status"}, dataflattentable.WithIncludeStderr()),
		zfs.ZfsPropertiesPlugin(slogger),
		zfs.ZpoolPropertiesPlugin(slogger),
	}
}

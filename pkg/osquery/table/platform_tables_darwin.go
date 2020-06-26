// +build darwin

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/knightsc/system_policy/osquery/table/kextpolicy"
	"github.com/knightsc/system_policy/osquery/table/legacyexec"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/firmwarepasswd"
	"github.com/kolide/launcher/pkg/osquery/tables/munki"
	"github.com/kolide/launcher/pkg/osquery/tables/screenlock"
	"github.com/kolide/launcher/pkg/osquery/tables/systemprofiler"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	_ "github.com/mattn/go-sqlite3"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	munki := munki.New()

	return []*table.Plugin{
		Airdrop(client),
		AppIcons(),
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
		kextpolicy.TablePlugin(),
		legacyexec.TablePlugin(),
		dataflattentable.TablePlugin(client, logger, dataflattentable.PlistType),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_pwpolicy", dataflattentable.PlistType, []string{"/usr/bin/pwpolicy", "getaccountpolicies"}),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_apfs_users", dataflattentable.PlistType, []string{"/usr/sbin/diskutil", "apfs", "listUsers", "/", "-plist"}),
		screenlock.TablePlugin(client, logger, currentOsquerydBinaryPath),
		systemprofiler.TablePlugin(client, logger),
		munki.ManagedInstalls(client, logger),
		munki.MunkiReport(client, logger),
	}
}

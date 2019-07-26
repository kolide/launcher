// +build darwin

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/knightsc/system_policy/osquery/table/kextpolicy"
	"github.com/knightsc/system_policy/osquery/table/legacyexec"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	_ "github.com/mattn/go-sqlite3"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	munki := new(MunkiInfo)

	return []*table.Plugin{
		Airdrop(client),
		AppIcons(),
		ChromeLoginKeychainInfo(client, logger),
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
		munki.ManagedInstalls(client, logger),
		munki.MunkiReport(client, logger),
	}
}

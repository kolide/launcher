// +build darwin

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	_ "github.com/mattn/go-sqlite3"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	munki := new(MunkiInfo)
	return []*table.Plugin{
		munki.MunkiReport(client, logger),
		munki.ManagedInstalls(client, logger),
		MDMInfo(logger),
		MachoInfo(),
		MacOSUpdate(client),
		LauncherInfo(client),
		EmailAddresses(client, logger),
		Spotlight(),
		KolideVulnerabilities(client, logger),
		BestPractices(client),
		Airdrop(client),
		ChromeLoginKeychainInfo(client, logger),
		GDriveSyncConfig(client, logger),
		GDriveSyncHistoryInfo(client, logger),
	}
}

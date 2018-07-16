// +build darwin

package osqtable

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
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
		EmailAddresses(client),
		Spotlight(),
		KolideVulnerabilities(client, logger),
		BestPractices(client),
		Airdrop(client),
	}
}

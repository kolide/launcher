// +build darwin

package osquery

import (
	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	munki := new(MunkiInfo)
	return []*table.Plugin{
		LauncherInfo(client),
		BestPractices(client),
		EmailAddresses(client),
		Spotlight(),
		munki.MunkiReport(client, logger),
		munki.ManagedInstalls(client, logger),
		MDMInfo(logger),
		KolideVulnerabilities(client, logger),
	}
}

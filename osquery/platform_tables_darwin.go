// +build darwin

package osquery

import (
	"github.com/kolide/launcher/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	return []*table.Plugin{
		LauncherInfo(client),
		BestPractices(client),
		EmailAddresses(client),
		Spotlight(),
		KolideVulnerabilities(client, logger),
	}
}

// +build darwin

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"

	"github.com/kolide/launcher/osquery/table/email"
	"github.com/kolide/launcher/osquery/table/launcher"
	"github.com/kolide/launcher/osquery/table/macho"
	"github.com/kolide/launcher/osquery/table/mdm"
	"github.com/kolide/launcher/osquery/table/munki"
	"github.com/kolide/launcher/osquery/table/osupdate"
	"github.com/kolide/launcher/osquery/table/practice"
	"github.com/kolide/launcher/osquery/table/spotlight"
	"github.com/kolide/launcher/osquery/table/vulnerability"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	munki := new(munki.MunkiInfo)
	return []*table.Plugin{
		munki.MunkiReport(client, logger),
		munki.ManagedInstalls(client, logger),
		mdm.Info(logger),
		macho.Info(),
		osupdate.MacOS(client),
		launcher.LauncherInfo(client),
		email.Addresses(client),
		spotlight.Spotlight(),
		vulnerability.KolideVulnerabilities(client, logger),
		practice.BestPractices(client),
	}
}

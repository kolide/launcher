// +build !darwin

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"

	"github.com/kolide/launcher/osquery/table/email"
	"github.com/kolide/launcher/osquery/table/launcher"
	"github.com/kolide/launcher/osquery/table/practice"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	return []*table.Plugin{
		launcher.LauncherInfo(client),
		practice.BestPractices(client),
		email.Addresses(client),
	}
}

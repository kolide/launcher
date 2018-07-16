// +build !darwin

package osqtable

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	return []*table.Plugin{
		LauncherInfo(client),
		BestPractices(client),
		EmailAddresses(client),
	}
}

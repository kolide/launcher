// +build !darwin

package table

import (
	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	return []*table.Plugin{
		BestPractices(client),
		EmailAddresses(client, logger),
	}
}

// +build !darwin

package osquery

import (
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient) []*table.Plugin {
	return []*table.Plugin{
		BestPractices(client),
		EmailAddresses(client),
	}
}

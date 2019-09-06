// +build !darwin, !windows

package table

import (
	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

// platformTables returns an empty set. It's here as a catchall for
// unimplemented platforms.
func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	return []*table.Plugin{}
}

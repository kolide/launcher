// +build !darwin,!windows

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/gsettings"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

// platformTables returns an empty set. It's here as a catchall for
// unimplemented platforms.
func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	return []*table.Plugin{
		gsettings.TablePlugin(client, logger),
	}
}

// +build windows

package table

import (
	"github.com/kolide/launcher/pkg/osquery/tables/wmitable"

	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	return []*table.Plugin{
		ProgramIcons(),
		wmitable.TablePlugin(client, logger),
	}
}

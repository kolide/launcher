// +build linux

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/gsettings"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	return []*table.Plugin{
		gsettings.Settings(client, logger),
		gsettings.Metadata(client, logger),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_nmcli", dataflattentable.KeyValueType, []string{"/usr/bin/nmcli", "--mode=multiline", "--fields=all", "device", "wifi", "list"}, dataflattentable.WithKVSeparator(":")),
	}
}

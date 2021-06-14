// +build linux

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/cryptsetup"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/gsettings"
	"github.com/kolide/launcher/pkg/osquery/tables/xrdb"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	return []*table.Plugin{
		cryptsetup.TablePlugin(client, logger),
		gsettings.Settings(client, logger),
		gsettings.Metadata(client, logger),
		xrdb.TablePlugin(client, logger),
		dataflattentable.TablePluginExec(client, logger,
			"kolide_nmcli_wifi", dataflattentable.KeyValueType,
			[]string{"/usr/bin/nmcli", "--mode=multiline", "--fields=all", "device", "wifi", "list"},
			dataflattentable.WithKVSeparator(":")),
		dataflattentable.TablePluginExec(client, logger, "kolide_lsblk", dataflattentable.JsonType,
			[]string{"lsblk", "-J"},
			dataflattentable.WithBinDirs("/usr/bin", "/bin"),
		),
	}
}

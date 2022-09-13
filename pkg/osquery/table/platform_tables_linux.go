//go:build linux
// +build linux

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/cryptsetup"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/falcon_kernel_check"
	"github.com/kolide/launcher/pkg/osquery/tables/fscrypt_info"
	"github.com/kolide/launcher/pkg/osquery/tables/gsettings"
	"github.com/kolide/launcher/pkg/osquery/tables/secureboot"
	"github.com/kolide/launcher/pkg/osquery/tables/xrdb"
	osquery "github.com/osquery/osquery-go"
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		cryptsetup.TablePlugin(client, logger),
		gsettings.Settings(client, logger),
		gsettings.Metadata(client, logger),
		secureboot.TablePlugin(client, logger),
		xrdb.TablePlugin(client, logger),
		fscrypt_info.TablePlugin(logger),
		falcon_kernel_check.TablePlugin(logger),
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

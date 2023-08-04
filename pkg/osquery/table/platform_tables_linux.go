//go:build linux
// +build linux

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/crowdstrike/falcon_kernel_check"
	"github.com/kolide/launcher/pkg/osquery/tables/crowdstrike/falconctl"
	"github.com/kolide/launcher/pkg/osquery/tables/cryptsetup"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/apt"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/dnf"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/dpkg"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/pacman/group"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/pacman/info"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/pacman/upgradeable"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/rpm"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/simple_array"
	"github.com/kolide/launcher/pkg/osquery/tables/fscrypt_info"
	"github.com/kolide/launcher/pkg/osquery/tables/gsettings"
	"github.com/kolide/launcher/pkg/osquery/tables/secureboot"
	"github.com/kolide/launcher/pkg/osquery/tables/xfconf"
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
		falconctl.NewFalconctlOptionTable(logger),
		xfconf.TablePlugin(logger),

		dataflattentable.TablePluginExec(client, logger,
			"kolide_nmcli_wifi", dataflattentable.KeyValueType,
			[]string{"/usr/bin/nmcli", "--mode=multiline", "--fields=all", "device", "wifi", "list"},
			dataflattentable.WithKVSeparator(":")),
		dataflattentable.TablePluginExec(client, logger, "kolide_lsblk", dataflattentable.JsonType,
			[]string{"lsblk", "-J"},
			dataflattentable.WithBinDirs("/usr/bin", "/bin"),
		),
		dataflattentable.NewExecAndParseTable(logger, "kolide_falconctl_systags", simple_array.New("systags"), []string{"/opt/CrowdStrike/falconctl", "-g", "--systags"}),
		dataflattentable.NewExecAndParseTable(logger, "kolide_apt_upgradeable", apt.Parser, []string{"/usr/bin/apt", "list", "--upgradeable"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dnf_upgradeable", dnf.Parser, []string{"/usr/bin/dnf", "check-update"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dpkg_version_info", dpkg.Parser, []string{"/usr/bin/dpkg", "-p"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_group", pacman_group.Parser, []string{"/usr/bin/pacman", "-Qg"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_version_info", pacman_info.Parser, []string{"/usr/bin/pacman", "-Qi"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_upgradeable", pacman_upgradeable.Parser, []string{"/usr/bin/pacman", "-Qu"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_rpm_version_info", rpm.Parser, []string{"/usr/bin/rpm", "-qai"}, dataflattentable.WithIncludeStderr()),
	}
}

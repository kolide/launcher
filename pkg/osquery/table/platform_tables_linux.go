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
	pacman_group "github.com/kolide/launcher/pkg/osquery/tables/execparsers/pacman/group"
	pacman_info "github.com/kolide/launcher/pkg/osquery/tables/execparsers/pacman/info"
	pacman_upgradeable "github.com/kolide/launcher/pkg/osquery/tables/execparsers/pacman/upgradeable"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/repcli"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/rpm"
	"github.com/kolide/launcher/pkg/osquery/tables/execparsers/simple_array"
	"github.com/kolide/launcher/pkg/osquery/tables/fscrypt_info"
	"github.com/kolide/launcher/pkg/osquery/tables/gsettings"
	"github.com/kolide/launcher/pkg/osquery/tables/secureboot"
	"github.com/kolide/launcher/pkg/osquery/tables/xfconf"
	"github.com/kolide/launcher/pkg/osquery/tables/xrdb"
	osquery "github.com/osquery/osquery-go"
)

func platformTables(logger log.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		cryptsetup.TablePlugin(logger),
		gsettings.Settings(logger),
		gsettings.Metadata(logger),
		secureboot.TablePlugin(logger),
		xrdb.TablePlugin(logger),
		fscrypt_info.TablePlugin(logger),
		falcon_kernel_check.TablePlugin(logger),
		falconctl.NewFalconctlOptionTable(logger),
		xfconf.TablePlugin(logger),

		dataflattentable.TablePluginExec(logger,
			"kolide_nmcli_wifi", dataflattentable.KeyValueType,
			[]string{"nmcli", "--mode=multiline", "--fields=all", "device", "wifi", "list"},
			dataflattentable.WithKVSeparator(":")),
		dataflattentable.TablePluginExec(logger, "kolide_lsblk", dataflattentable.JsonType,
			[]string{"lsblk", "-J"},
		),
		dataflattentable.NewExecAndParseTable(logger, "kolide_falconctl_systags", simple_array.New("systags"), []string{"/opt/CrowdStrike/falconctl", "-g", "--systags"}),
		dataflattentable.NewExecAndParseTable(logger, "kolide_apt_upgradeable", apt.Parser, []string{"apt", "list", "--upgradeable"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dnf_upgradeable", dnf.Parser, []string{"dnf", "check-update"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dpkg_version_info", dpkg.Parser, []string{"dpkg", "-p"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_group", pacman_group.Parser, []string{"pacman", "-Qg"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_version_info", pacman_info.Parser, []string{"pacman", "-Qi"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_upgradeable", pacman_upgradeable.Parser, []string{"pacman", "-Qu"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_rpm_version_info", rpm.Parser, []string{"rpm", "-qai"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_carbonblack_repcli_status", repcli.Parser, []string{"/opt/carbonblack/psc/bin/repcli", "status"}, dataflattentable.WithIncludeStderr()),
	}
}

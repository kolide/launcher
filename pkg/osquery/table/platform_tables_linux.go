//go:build linux
// +build linux

package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/allowedcmd"
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
	"github.com/kolide/launcher/pkg/osquery/tables/zfs"
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
			allowedcmd.Nmcli,
			[]string{"--mode=multiline", "--fields=all", "device", "wifi", "list"},
			dataflattentable.WithKVSeparator(":")),
		dataflattentable.TablePluginExec(logger, "kolide_lsblk", dataflattentable.JsonType,
			allowedcmd.Lsblk, []string{"-J"},
		),
		dataflattentable.NewExecAndParseTable(logger, "kolide_falconctl_systags", simple_array.New("systags"), allowedcmd.Falconctl, []string{"-g", "--systags"}),
		dataflattentable.NewExecAndParseTable(logger, "kolide_apt_upgradeable", apt.Parser, allowedcmd.Apt, []string{"list", "--upgradeable"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dnf_upgradeable", dnf.Parser, allowedcmd.Dnf, []string{"check-update"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dpkg_version_info", dpkg.Parser, allowedcmd.Dpkg, []string{"-p"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_group", pacman_group.Parser, allowedcmd.Pacman, []string{"-Qg"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_version_info", pacman_info.Parser, allowedcmd.Pacman, []string{"-Qi"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_pacman_upgradeable", pacman_upgradeable.Parser, allowedcmd.Pacman, []string{"-Qu"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_rpm_version_info", rpm.Parser, allowedcmd.Rpm, []string{"-qai"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(logger, "kolide_carbonblack_repcli_status", repcli.Parser, allowedcmd.Repcli, []string{"status"}, dataflattentable.WithIncludeStderr()),
		zfs.ZfsPropertiesPlugin(logger),
		zfs.ZpoolPropertiesPlugin(logger),
	}
}

//go:build linux
// +build linux

package table

import (
	"log/slog"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/crowdstrike/falcon_kernel_check"
	"github.com/kolide/launcher/ee/tables/crowdstrike/falconctl"
	"github.com/kolide/launcher/ee/tables/cryptsetup"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/execparsers/apt"
	"github.com/kolide/launcher/ee/tables/execparsers/data_table"
	"github.com/kolide/launcher/ee/tables/execparsers/dnf"
	"github.com/kolide/launcher/ee/tables/execparsers/dpkg"
	"github.com/kolide/launcher/ee/tables/execparsers/flatpak/remote_ls/upgradeable"
	pacman_group "github.com/kolide/launcher/ee/tables/execparsers/pacman/group"
	pacman_info "github.com/kolide/launcher/ee/tables/execparsers/pacman/info"
	pacman_upgradeable "github.com/kolide/launcher/ee/tables/execparsers/pacman/upgradeable"
	"github.com/kolide/launcher/ee/tables/execparsers/repcli"
	"github.com/kolide/launcher/ee/tables/execparsers/rpm"
	"github.com/kolide/launcher/ee/tables/execparsers/simple_array"
	"github.com/kolide/launcher/ee/tables/fscrypt_info"
	"github.com/kolide/launcher/ee/tables/gsettings"
	"github.com/kolide/launcher/ee/tables/homebrew"
	nix_env_upgradeable "github.com/kolide/launcher/ee/tables/nix_env/upgradeable"
	"github.com/kolide/launcher/ee/tables/secureboot"
	"github.com/kolide/launcher/ee/tables/xfconf"
	"github.com/kolide/launcher/ee/tables/xrdb"
	"github.com/kolide/launcher/ee/tables/zfs"
	osquery "github.com/osquery/osquery-go"
)

func platformSpecificTables(slogger *slog.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		brew_upgradeable.TablePlugin(slogger),
		cryptsetup.TablePlugin(slogger),
		gsettings.Settings(slogger),
		gsettings.Metadata(slogger),
		nix_env_upgradeable.TablePlugin(slogger),
		secureboot.TablePlugin(slogger),
		xrdb.TablePlugin(slogger),
		fscrypt_info.TablePlugin(slogger),
		falcon_kernel_check.TablePlugin(slogger),
		falconctl.NewFalconctlOptionTable(slogger),
		xfconf.TablePlugin(slogger),

		dataflattentable.TablePluginExec(slogger,
			"kolide_nmcli_wifi", dataflattentable.KeyValueType,
			allowedcmd.Nmcli,
			[]string{"--mode=multiline", "--fields=all", "device", "wifi", "list"},
			dataflattentable.WithKVSeparator(":")),
		dataflattentable.TablePluginExec(slogger, "kolide_lsblk", dataflattentable.JsonType,
			allowedcmd.Lsblk, []string{"-fJp"},
		),
		dataflattentable.TablePluginExec(slogger, "kolide_wsone_uem_status_enroll", dataflattentable.JsonType, allowedcmd.Ws1HubUtil, []string{"status", "--enroll"}),
		dataflattentable.TablePluginExec(slogger, "kolide_wsone_uem_status_dependency", dataflattentable.JsonType, allowedcmd.Ws1HubUtil, []string{"status", "--dependency"}),
		dataflattentable.TablePluginExec(slogger, "kolide_wsone_uem_status_profile", dataflattentable.JsonType, allowedcmd.Ws1HubUtil, []string{"status", "--profile"}),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_falconctl_systags", simple_array.New("systags"), allowedcmd.Falconctl, []string{"-g", "--systags"}),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_apt_upgradeable", apt.Parser, allowedcmd.Apt, []string{"list", "--upgradeable"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_dnf_upgradeable", dnf.Parser, allowedcmd.Dnf, []string{"check-update"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_dpkg_version_info", dpkg.Parser, allowedcmd.Dpkg, []string{"-p"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_flatpak_upgradeable", flatpak_upgradeable.Parser, allowedcmd.Flatpak, []string{"remote-ls", "--updates"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_pacman_group", pacman_group.Parser, allowedcmd.Pacman, []string{"-Qg"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_pacman_version_info", pacman_info.Parser, allowedcmd.Pacman, []string{"-Qi"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_pacman_upgradeable", pacman_upgradeable.Parser, allowedcmd.Pacman, []string{"-Qu"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_rpm_version_info", rpm.Parser, allowedcmd.Rpm, []string{"-qai"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_snap_installed", data_table.NewParser(), allowedcmd.Snap, []string{"list"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_snap_upgradeable", data_table.NewParser(), allowedcmd.Snap, []string{"refresh", "--list"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_carbonblack_repcli_status", repcli.Parser, allowedcmd.Repcli, []string{"status"}, dataflattentable.WithIncludeStderr()),
		dataflattentable.TablePluginExec(slogger, "kolide_zypper_upgradeable_packages", dataflattentable.XmlType, allowedcmd.Zypper, []string{"-x", "lu"}),
		dataflattentable.TablePluginExec(slogger, "kolide_zypper_upgradeable_patches", dataflattentable.XmlType, allowedcmd.Zypper, []string{"-x", "lp"}),
		dataflattentable.TablePluginExec(slogger, "kolide_nftables", dataflattentable.JsonType, allowedcmd.Nftables, []string{"-jat", "list", "ruleset"}), // -j (json) -a (show object handles) -t (terse, omit set contents)
		zfs.ZfsPropertiesPlugin(slogger),
		zfs.ZpoolPropertiesPlugin(slogger),
	}
}

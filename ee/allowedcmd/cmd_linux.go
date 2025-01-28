//go:build linux
// +build linux

package allowedcmd

import (
	"context"
	"errors"
)

func Apt(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/apt", arg...)
}

func Brew(ctx context.Context, arg ...string) (*TracedCmd, error) {
	validatedCmd, err := validatedCommand(ctx, "/home/linuxbrew/.linuxbrew/bin/brew", arg...)
	if err != nil {
		return nil, err
	}

	validatedCmd.Env = append(validatedCmd.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")

	return validatedCmd, nil
}

func Coredumpctl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/coredumpctl", arg...)
}

func Cryptsetup(ctx context.Context, arg ...string) (*TracedCmd, error) {
	for _, p := range []string{"/usr/sbin/cryptsetup", "/sbin/cryptsetup"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		return validatedCmd, nil
	}

	return nil, errors.New("cryptsetup not found")
}

func Dnf(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/dnf", arg...)
}

func Dpkg(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/dpkg", arg...)
}

func Echo(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/echo", arg...)
}

func Falconctl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/opt/CrowdStrike/falconctl", arg...)
}

func FalconKernelCheck(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/opt/CrowdStrike/falcon-kernel-check", arg...)
}

func Flatpak(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/flatpak", arg...)
}

func GnomeExtensions(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/gnome-extensions", arg...)
}

func Gsettings(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/gsettings", arg...)
}

func Ifconfig(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/ifconfig", arg...)
}

func Ip(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/ip", arg...)
}

func Journalctl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/journalctl", arg...)
}

func Loginctl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/loginctl", arg...)
}

func Lsblk(ctx context.Context, arg ...string) (*TracedCmd, error) {
	for _, p := range []string{"/bin/lsblk", "/usr/bin/lsblk"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		return validatedCmd, nil
	}

	return nil, errors.New("lsblk not found")
}

func Lsof(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/lsof", arg...)
}

func NixEnv(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/run/current-system/sw/bin/nix-env", arg...)
}

func Nftables(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/nft", arg...)
}

func Nmcli(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/nmcli", arg...)
}

func NotifySend(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/notify-send", arg...)
}

func Pacman(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/pacman", arg...)
}

func Patchelf(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/run/current-system/sw/bin/patchelf", arg...)
}

func Ps(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/ps", arg...)
}

func Repcli(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/opt/carbonblack/psc/bin/repcli", arg...)
}

func Rpm(ctx context.Context, arg ...string) (*TracedCmd, error) {
	for _, p := range []string{"/bin/rpm", "/usr/bin/rpm"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		return validatedCmd, nil
	}

	return nil, errors.New("rpm not found")
}

func Snap(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/snap", arg...)
}

func Systemctl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/systemctl", arg...)
}

func Ws1HubUtil(ctx context.Context, arg ...string) (*TracedCmd, error) {
	for _, p := range []string{"/usr/bin/ws1HubUtil", "/opt/vmware/ws1-hub/bin/ws1HubUtil"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		return validatedCmd, nil
	}

	return nil, errors.New("ws1HubUtil not found")
}

func XdgOpen(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/xdg-open", arg...)
}

func Xrdb(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/xrdb", arg...)
}

func XWwwBrowser(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/x-www-browser", arg...)
}

func ZerotierCli(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/local/bin/zerotier-cli", arg...)
}

func Zfs(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/zfs", arg...)
}

func Zpool(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/zpool", arg...)
}

func Zypper(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/zypper", arg...)
}

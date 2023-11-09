//go:build linux
// +build linux

package allowedcmd

import (
	"context"
	"errors"
	"os/exec"
)

func Apt(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/apt", arg...)
}

func Cryptsetup(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	for _, p := range []string{"/usr/sbin/cryptsetup", "/sbin/cryptsetup"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		return validatedCmd, nil
	}

	return nil, errors.New("cryptsetup not found")
}

func Dnf(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/dnf", arg...)
}

func Dpkg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/dpkg", arg...)
}

func Echo(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/echo", arg...)
}

func Falconctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/opt/CrowdStrike/falconctl", arg...)
}

func Falconkernelcheck(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/opt/CrowdStrike/falcon-kernel-check", arg...)
}

func Gnomeextensions(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/gnome-extensions", arg...)
}

func Gsettings(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/gsettings", arg...)
}

func Ifconfig(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/sbin/ifconfig", arg...)
}

func Ip(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/sbin/ip", arg...)
}

func Loginctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/loginctl", arg...)
}

func Lsblk(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	for _, p := range []string{"/bin/lsblk", "/usr/bin/lsblk"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		return validatedCmd, nil
	}

	return nil, errors.New("lsblk not found")
}

func Lsof(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/lsof", arg...)
}

func Nmcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/nmcli", arg...)
}

func Notifysend(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/notify-send", arg...)
}

func Pacman(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/pacman", arg...)
}

func Ps(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/ps", arg...)
}

func Repcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/opt/carbonblack/psc/bin/repcli", arg...)
}

func Rpm(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	for _, p := range []string{"/bin/rpm", "/usr/bin/rpm"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		return validatedCmd, nil
	}

	return nil, errors.New("rpm not found")
}

func Systemctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/systemctl", arg...)
}

func Xdgopen(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/xdg-open", arg...)
}

func Xrdb(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/xrdb", arg...)
}

func Xwwwbrowser(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/bin/x-www-browser", arg...)
}

func Zerotiercli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/local/bin/zerotier-cli", arg...)
}

func Zfs(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/sbin/zfs", arg...)
}

func Zpool(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, "/usr/sbin/zpool", arg...)
}

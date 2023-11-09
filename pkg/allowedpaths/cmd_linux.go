//go:build linux
// +build linux

package allowedpaths

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

func Apt(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/apt")
	if err != nil {
		return nil, fmt.Errorf("apt not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Cryptsetup(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	for _, p := range []string{"/usr/sbin/cryptsetup", "/sbin/cryptsetup"} {
		fullPathToCmdValidated, err := validatedPath(p)
		if err != nil {
			continue
		}

		return newCmd(ctx, fullPathToCmdValidated, arg...), nil
	}

	return nil, errors.New("cryptsetup not found")
}

func Dnf(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/dnf")
	if err != nil {
		return nil, fmt.Errorf("dnf not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Dpkg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/dpkg")
	if err != nil {
		return nil, fmt.Errorf("dpkg not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Echo(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/echo")
	if err != nil {
		return nil, fmt.Errorf("echo not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Falconctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/opt/CrowdStrike/falconctl")
	if err != nil {
		return nil, fmt.Errorf("falconctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Falconkernelcheck(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/opt/CrowdStrike/falcon-kernel-check")
	if err != nil {
		return nil, fmt.Errorf("falcon-kernel-check not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Gnomeextensions(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/gnome-extensions")
	if err != nil {
		return nil, fmt.Errorf("gnome-extensions not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Gsettings(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/gsettings")
	if err != nil {
		return nil, fmt.Errorf("gsettings not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ifconfig(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/ifconfig")
	if err != nil {
		return nil, fmt.Errorf("ifconfig not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ip(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/ip")
	if err != nil {
		return nil, fmt.Errorf("ip not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Loginctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/loginctl")
	if err != nil {
		return nil, fmt.Errorf("loginctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Lsblk(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	for _, p := range []string{"/bin/lsblk", "/usr/bin/lsblk"} {
		fullPathToCmdValidated, err := validatedPath(p)
		if err != nil {
			continue
		}

		return newCmd(ctx, fullPathToCmdValidated, arg...), nil
	}

	return nil, errors.New("lsblk not found")
}

func Lsof(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/lsof")
	if err != nil {
		return nil, fmt.Errorf("lsof not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Nmcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/nmcli")
	if err != nil {
		return nil, fmt.Errorf("nmcli not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Notifysend(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/notify-send")
	if err != nil {
		return nil, fmt.Errorf("notify-send not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Pacman(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/pacman")
	if err != nil {
		return nil, fmt.Errorf("pacman not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ps(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/ps")
	if err != nil {
		return nil, fmt.Errorf("ps not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Repcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/opt/carbonblack/psc/bin/repcli")
	if err != nil {
		return nil, fmt.Errorf("repcli not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Rpm(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	for _, p := range []string{"/bin/rpm", "/usr/bin/rpm"} {
		fullPathToCmdValidated, err := validatedPath(p)
		if err != nil {
			continue
		}

		return newCmd(ctx, fullPathToCmdValidated, arg...), nil
	}

	return nil, errors.New("rpm not found")
}

func Systemctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/systemctl")
	if err != nil {
		return nil, fmt.Errorf("systemctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Xdgopen(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/xdg-open")
	if err != nil {
		return nil, fmt.Errorf("xdg-open not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Xrdb(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/xrdb")
	if err != nil {
		return nil, fmt.Errorf("xrdb not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Xwwwbrowser(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/x-www-browser")
	if err != nil {
		return nil, fmt.Errorf("x-www-browser not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Zerotiercli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/local/bin/zerotier-cli")
	if err != nil {
		return nil, fmt.Errorf("zerotier-cli not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Zfs(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/zfs")
	if err != nil {
		return nil, fmt.Errorf("zfs not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Zpool(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/zpool")
	if err != nil {
		return nil, fmt.Errorf("zpool not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

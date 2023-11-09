//go:build darwin
// +build darwin

package allowedpaths

import (
	"context"
	"fmt"
	"os/exec"
)

func Airport(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport")
	if err != nil {
		return nil, fmt.Errorf("airport not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Bioutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/bioutil")
	if err != nil {
		return nil, fmt.Errorf("bioutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Bputil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/bputil")
	if err != nil {
		return nil, fmt.Errorf("bputil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Diskutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/diskutil")
	if err != nil {
		return nil, fmt.Errorf("diskutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Echo(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/bin/echo")
	if err != nil {
		return nil, fmt.Errorf("echo not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Falconctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/Applications/Falcon.app/Contents/Resources/falconctl")
	if err != nil {
		return nil, fmt.Errorf("falconctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Fdesetup(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/fdesetup")
	if err != nil {
		return nil, fmt.Errorf("fdesetup not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Firmwarepasswd(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/firmwarepasswd")
	if err != nil {
		return nil, fmt.Errorf("firmwarepasswd not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ifconfig(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/sbin/ifconfig")
	if err != nil {
		return nil, fmt.Errorf("ifconfig not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ioreg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/ioreg")
	if err != nil {
		return nil, fmt.Errorf("ioreg not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Launchctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/bin/launchctl")
	if err != nil {
		return nil, fmt.Errorf("launchctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Lsof(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/lsof")
	if err != nil {
		return nil, fmt.Errorf("lsof not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Mdfind(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/mdfind")
	if err != nil {
		return nil, fmt.Errorf("mdfind not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Mdmclient(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/libexec/mdmclient")
	if err != nil {
		return nil, fmt.Errorf("mdmclient not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Netstat(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/netstat")
	if err != nil {
		return nil, fmt.Errorf("netstat not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Open(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/open")
	if err != nil {
		return nil, fmt.Errorf("open not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Pkgutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/pkgutil")
	if err != nil {
		return nil, fmt.Errorf("pkgutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Powermetrics(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/powermetrics")
	if err != nil {
		return nil, fmt.Errorf("powermetrics not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Profiles(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/profiles")
	if err != nil {
		return nil, fmt.Errorf("profiles not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ps(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/bin/ps")
	if err != nil {
		return nil, fmt.Errorf("ps not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Pwpolicy(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/pwpolicy")
	if err != nil {
		return nil, fmt.Errorf("pwpolicy not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Remotectl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/libexec/remotectl")
	if err != nil {
		return nil, fmt.Errorf("remotectl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Repcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/Applications/VMware Carbon Black Cloud/repcli.bundle/Contents/MacOS/repcli")
	if err != nil {
		return nil, fmt.Errorf("repcli not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Scutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/scutil")
	if err != nil {
		return nil, fmt.Errorf("scutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Softwareupdate(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/softwareupdate")
	if err != nil {
		return nil, fmt.Errorf("softwareupdate not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Systemprofiler(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/sbin/system_profiler")
	if err != nil {
		return nil, fmt.Errorf("system_profiler not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Tmutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath("/usr/bin/tmutil")
	if err != nil {
		return nil, fmt.Errorf("tmutil not found: %w", err)
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

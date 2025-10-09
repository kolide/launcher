//go:build darwin
// +build darwin

package allowedcmd

import (
	"context"
	"fmt"
)

func Airport(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport", arg...)
}

func Bioutil(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/bioutil", arg...)
}

func Bputil(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/bputil", arg...)
}

func Brew(ctx context.Context, arg ...string) (*TracedCmd, error) {
	for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
		validatedCmd, err := validatedCommand(ctx, p, arg...)
		if err != nil {
			continue
		}

		validatedCmd.Env = append(validatedCmd.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")

		return validatedCmd, nil
	}

	return nil, fmt.Errorf("%w: homebrew", ErrCommandNotFound)
}

func Diskutil(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/diskutil", arg...)
}

func Echo(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/bin/echo", arg...)
}

func Falconctl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/Applications/Falcon.app/Contents/Resources/falconctl", arg...)
}

func Fdesetup(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/fdesetup", arg...)
}

func Firmwarepasswd(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/firmwarepasswd", arg...)
}

func Ifconfig(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/sbin/ifconfig", arg...)
}

func Ioreg(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/ioreg", arg...)
}

func Launchctl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/bin/launchctl", arg...)
}

func Lsof(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/lsof", arg...)
}

func Mdfind(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/mdfind", arg...)
}

func Mdmclient(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/libexec/mdmclient", arg...)
}

func MicrosoftDefenderATP(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/local/bin/mdatp", arg...)
}

func Netstat(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/netstat", arg...)
}

func NixEnv(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/nix/var/nix/profiles/default/bin/nix-env", arg...)
}

func Open(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/open", arg...)
}

func Pkgutil(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/pkgutil", arg...)
}

func Powermetrics(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/powermetrics", arg...)
}

func Profiles(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/profiles", arg...)
}

func Ps(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/bin/ps", arg...)
}

func Pwpolicy(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/pwpolicy", arg...)
}

func Remotectl(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/libexec/remotectl", arg...)
}

func Repcli(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/Applications/VMware Carbon Black Cloud/repcli.bundle/Contents/MacOS/repcli", arg...)
}

func Scutil(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/scutil", arg...)
}

func Security(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/security", arg...)
}

func Socketfilterfw(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/libexec/ApplicationFirewall/socketfilterfw", arg...)
}

func Softwareupdate(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/softwareupdate", arg...)
}

func SystemProfiler(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/sbin/system_profiler", arg...)
}

func Tmutil(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/usr/bin/tmutil", arg...)
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

func Zscli(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return validatedCommand(ctx, "/Applications/Zscaler/Zscaler.app/Contents/PlugIns/zscli", arg...)
}

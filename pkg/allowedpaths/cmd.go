package allowedpaths

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
)

type AllowedCommand func(ctx context.Context, arg ...string) (*exec.Cmd, error)

func Airport(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("airport supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport")
	if err != nil {
		return nil, fmt.Errorf("airport not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Apt(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("apt supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/apt")
	if err != nil {
		return nil, fmt.Errorf("apt not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Bioutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("bioutil supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/bioutil")
	if err != nil {
		return nil, fmt.Errorf("bioutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Bputil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("bputil supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/bputil")
	if err != nil {
		return nil, fmt.Errorf("bputil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Commandprompt(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("cmd.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\cmd.exe`)
	if err != nil {
		return nil, fmt.Errorf("cmd.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Cryptsetup(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("cryptsetup supported on linux only")
	}

	for _, p := range []string{"/usr/sbin/cryptsetup", "/sbin/cryptsetup"} {
		fullPathToCmdValidated, err := validatedPath(p)
		if err != nil {
			continue
		}

		return newCmd(ctx, fullPathToCmdValidated, arg...), nil
	}

	return nil, errors.New("cryptsetup not found")
}

func Diskutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("diskutil supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/diskutil")
	if err != nil {
		return nil, fmt.Errorf("diskutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Dism(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("dism.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\Dism.exe`)
	if err != nil {
		return nil, fmt.Errorf("dism.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Dnf(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("dnf supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/dnf")
	if err != nil {
		return nil, fmt.Errorf("dnf not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Dpkg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("dpkg supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/dpkg")
	if err != nil {
		return nil, fmt.Errorf("dpkg not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Dsregcmd(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("dsregcmd.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\dsregcmd.exe`)
	if err != nil {
		return nil, fmt.Errorf("dsregcmd.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Echo(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	// echo on Windows is only available as a command in cmd.exe
	if runtime.GOOS == "windows" {
		return newCmd(ctx, "echo", arg...), nil
	}

	var fullPathToCmd string
	switch runtime.GOOS {
	case "darwin":
		fullPathToCmd = "/bin/echo"
	case "linux":
		fullPathToCmd = "/usr/bin/echo"
	}

	fullPathToCmdValidated, err := validatedPath(fullPathToCmd)
	if err != nil {
		return nil, fmt.Errorf("echo not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Falconctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("falconctl supported on darwin and linux only")
	}

	var fullPathToCmd string
	switch runtime.GOOS {
	case "darwin":
		fullPathToCmd = "/Applications/Falcon.app/Contents/Resources/falconctl"
	case "linux":
		fullPathToCmd = "/opt/CrowdStrike/falconctl"
	}

	fullPathToCmdValidated, err := validatedPath(fullPathToCmd)
	if err != nil {
		return nil, fmt.Errorf("falconctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Falconkernelcheck(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("falcon-kernel-check supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/opt/CrowdStrike/falcon-kernel-check")
	if err != nil {
		return nil, fmt.Errorf("falcon-kernel-check not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Fdesetup(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("fdesetup supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/fdesetup")
	if err != nil {
		return nil, fmt.Errorf("fdesetup not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Firmwarepasswd(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("firmwarepasswd supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/firmwarepasswd")
	if err != nil {
		return nil, fmt.Errorf("firmwarepasswd not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Gnomeextensions(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("gnome-extensions supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/gnome-extensions")
	if err != nil {
		return nil, fmt.Errorf("gnome-extensions not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Gsettings(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("gsettings supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/gsettings")
	if err != nil {
		return nil, fmt.Errorf("gsettings not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ifconfig(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("ifconfig supported on darwin and linux only")
	}

	var fullPathToCmd string
	switch runtime.GOOS {
	case "darwin":
		fullPathToCmd = "/sbin/ifconfig"
	case "linux":
		fullPathToCmd = "/usr/sbin/ifconfig"
	}

	fullPathToCmdValidated, err := validatedPath(fullPathToCmd)
	if err != nil {
		return nil, fmt.Errorf("ifconfig not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ioreg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("ioreg supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/ioreg")
	if err != nil {
		return nil, fmt.Errorf("ioreg not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ip(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("ip supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/ip")
	if err != nil {
		return nil, fmt.Errorf("ip not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ipconfig(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("ipconfig.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\ipconfig.exe`)
	if err != nil {
		return nil, fmt.Errorf("ipconfig.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Launchctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("launchctl supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/bin/launchctl")
	if err != nil {
		return nil, fmt.Errorf("launchctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Loginctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("loginctl supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/loginctl")
	if err != nil {
		return nil, fmt.Errorf("loginctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Lsblk(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("lsblk supported on linux only")
	}

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
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("lsof supported on darwin and linux only")
	}

	var fullPathToCmd string
	switch runtime.GOOS {
	case "darwin":
		fullPathToCmd = "/usr/sbin/lsof"
	case "linux":
		fullPathToCmd = "/usr/bin/lsof"
	}

	fullPathToCmdValidated, err := validatedPath(fullPathToCmd)
	if err != nil {
		return nil, fmt.Errorf("lsof not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Mdfind(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("mdfind supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/mdfind")
	if err != nil {
		return nil, fmt.Errorf("mdfind not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Mdmclient(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("mdmclient supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/libexec/mdmclient")
	if err != nil {
		return nil, fmt.Errorf("mdmclient not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Netstat(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("netstat supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/netstat")
	if err != nil {
		return nil, fmt.Errorf("netstat not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Nmcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("nmcli supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/nmcli")
	if err != nil {
		return nil, fmt.Errorf("nmcli not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Notifysend(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("notify-send supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/notify-send")
	if err != nil {
		return nil, fmt.Errorf("notify-send not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Open(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("open supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/open")
	if err != nil {
		return nil, fmt.Errorf("open not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Pacman(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("pacman supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/pacman")
	if err != nil {
		return nil, fmt.Errorf("pacman not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Pkgutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("pkgutil supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/pkgutil")
	if err != nil {
		return nil, fmt.Errorf("pkgutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Powercfg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("powercfg.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\powercfg.exe`)
	if err != nil {
		return nil, fmt.Errorf("powercfg.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Powermetrics(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("powermetrics supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/powermetrics")
	if err != nil {
		return nil, fmt.Errorf("powermetrics not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Powershell(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("powershell.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`)
	if err != nil {
		return nil, fmt.Errorf("powershell.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Profiles(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("profiles supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/profiles")
	if err != nil {
		return nil, fmt.Errorf("profiles not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Ps(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("ps supported on darwin and linux only")
	}

	var fullPathToCmd string
	switch runtime.GOOS {
	case "darwin":
		fullPathToCmd = "/bin/ps"
	case "linux":
		fullPathToCmd = "/usr/bin/ps"
	}

	fullPathToCmdValidated, err := validatedPath(fullPathToCmd)
	if err != nil {
		return nil, fmt.Errorf("ps not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Pwpolicy(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("pwpolicy supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/pwpolicy")
	if err != nil {
		return nil, fmt.Errorf("pwpolicy not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Remotectl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("remotectl supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/libexec/remotectl")
	if err != nil {
		return nil, fmt.Errorf("remotectl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Repcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	var fullPathToCmd string
	switch runtime.GOOS {
	case "darwin":
		fullPathToCmd = "/Applications/VMware Carbon Black Cloud/repcli.bundle/Contents/MacOS/repcli"
	case "linux":
		fullPathToCmd = "/opt/carbonblack/psc/bin/repcli"
	case "windows":
		fullPathToCmd = filepath.Join("Program Files", "Confer", "repcli")
	}

	fullPathToCmdValidated, err := validatedPath(fullPathToCmd)
	if err != nil {
		return nil, fmt.Errorf("repcli not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Rpm(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("rpm supported on linux only")
	}

	for _, p := range []string{"/bin/rpm", "/usr/bin/rpm"} {
		fullPathToCmdValidated, err := validatedPath(p)
		if err != nil {
			continue
		}

		return newCmd(ctx, fullPathToCmdValidated, arg...), nil
	}

	return nil, errors.New("rpm not found")
}

func Scutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("scutil supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/scutil")
	if err != nil {
		return nil, fmt.Errorf("scutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Secedit(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("secedit.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\SecEdit.exe`)
	if err != nil {
		return nil, fmt.Errorf("secedit.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Softwareupdate(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("softwareupdate supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/softwareupdate")
	if err != nil {
		return nil, fmt.Errorf("softwareupdate not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Systemctl(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("systemctl supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/systemctl")
	if err != nil {
		return nil, fmt.Errorf("systemctl not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Systemprofiler(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("system_profiler supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/system_profiler")
	if err != nil {
		return nil, fmt.Errorf("system_profiler not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Taskkill(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("taskkill.exe supported on windows only")
	}

	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\taskkill.exe`)
	if err != nil {
		return nil, fmt.Errorf("taskkill.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Tmutil(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("tmutil supported on darwin only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/tmutil")
	if err != nil {
		return nil, fmt.Errorf("tmutil not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Xdgopen(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("xdg-open supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/xdg-open")
	if err != nil {
		return nil, fmt.Errorf("xdg-open not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Xrdb(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("xrdb supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/xrdb")
	if err != nil {
		return nil, fmt.Errorf("xrdb not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Xwwwbrowser(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("x-www-browser supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/bin/x-www-browser")
	if err != nil {
		return nil, fmt.Errorf("x-www-browser not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Zerotiercli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	var fullPathToCmd string
	switch runtime.GOOS {
	case "darwin", "linux":
		fullPathToCmd = "/usr/local/bin/zerotier-cli"
	case "windows":
		fullPathToCmd = path.Join(os.Getenv("SYSTEMROOT"), "ProgramData", "ZeroTier", "One", "zerotier-one_x64.exe")
	}

	fullPathToCmdValidated, err := validatedPath(fullPathToCmd)
	if err != nil {
		return nil, fmt.Errorf("zerotier-cli not found: %w", err)
	}

	// For windows, "-q" should be prepended before all other args
	if runtime.GOOS == "windows" {
		arg = append([]string{"-q"}, arg...)
	}

	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Zfs(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("zfs supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/zfs")
	if err != nil {
		return nil, fmt.Errorf("zfs not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Zpool(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("zpool supported on linux only")
	}

	fullPathToCmdValidated, err := validatedPath("/usr/sbin/zpool")
	if err != nil {
		return nil, fmt.Errorf("zpool not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func newCmd(ctx context.Context, fullPathToCmd string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, fullPathToCmd, arg...) //nolint:forbidigo
}

func validatedPath(knownPath string) (string, error) {
	knownPath = filepath.Clean(knownPath)

	if _, err := os.Stat(knownPath); err == nil {
		return knownPath, nil
	}

	// Not found at known location -- return error for darwin and windows.
	// We expect to know the exact location for allowlisted commands on all
	// OSes except for a few Linux distros.
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("not found: %s", knownPath)
	}

	cmdName := filepath.Base(knownPath)
	if foundPath, err := exec.LookPath(cmdName); err == nil {
		return foundPath, nil
	}

	return "", fmt.Errorf("%s not found at %s and could not be located elsewhere", cmdName, knownPath)
}

//go:build linux

package allowedcmd

var Apt = newAllowedCommand("/usr/bin/apt")

var Brew = newAllowedCommand("/home/linuxbrew/.linuxbrew/bin/brew").WithEnv("HOMEBREW_NO_AUTO_UPDATE=1")

var Coredumpctl = newAllowedCommand("/usr/bin/coredumpctl")

var Cryptsetup = newAllowedCommand("/usr/sbin/cryptsetup", "/sbin/cryptsetup")

var Dnf = newAllowedCommand("/usr/bin/dnf")

var Dpkg = newAllowedCommand("/usr/bin/dpkg")

var Echo = newAllowedCommand("/usr/bin/echo")

var Falconctl = newAllowedCommand("/opt/CrowdStrike/falconctl")

var FalconKernelCheck = newAllowedCommand("/opt/CrowdStrike/falcon-kernel-check")

var Flatpak = newAllowedCommand("/usr/bin/flatpak")

var GnomeExtensions = newAllowedCommand("/usr/bin/gnome-extensions")

var Gsettings = newAllowedCommand("/usr/bin/gsettings")

var Ifconfig = newAllowedCommand("/usr/sbin/ifconfig")

var Ip = newAllowedCommand("/usr/sbin/ip")

var Journalctl = newAllowedCommand("/usr/bin/journalctl")

var Loginctl = newAllowedCommand("/usr/bin/loginctl")

var Lsblk = newAllowedCommand("/bin/lsblk", "/usr/bin/lsblk")

var Lsof = newAllowedCommand("/usr/bin/lsof")

var MicrosoftDefenderATP = newAllowedCommand("/usr/bin/mdatp")

var NixEnv = newAllowedCommand("/run/current-system/sw/bin/nix-env")

var Nftables = newAllowedCommand("/usr/sbin/nft")

var Nmcli = newAllowedCommand("/usr/bin/nmcli")

var NotifySend = newAllowedCommand("/usr/bin/notify-send")

var Pacman = newAllowedCommand("/usr/bin/pacman")

var Patchelf = newAllowedCommand("/run/current-system/sw/bin/patchelf")

var Ps = newAllowedCommand("/usr/bin/ps")

var Repcli = newAllowedCommand("/opt/carbonblack/psc/bin/repcli")

var Rpm = newAllowedCommand("/bin/rpm", "/usr/bin/rpm")

var Snap = newAllowedCommand("/usr/bin/snap")

var Systemctl = newAllowedCommand("/usr/bin/systemctl")

var Ws1HubUtil = newAllowedCommand("/usr/bin/ws1HubUtil", "/opt/vmware/ws1-hub/bin/ws1HubUtil")

var XdgOpen = newAllowedCommand("/usr/bin/xdg-open")

var Xrdb = newAllowedCommand("/usr/bin/xrdb")

var XWwwBrowser = newAllowedCommand("/usr/bin/x-www-browser")

var ZerotierCli = newAllowedCommand("/usr/local/bin/zerotier-cli")

var Zfs = newAllowedCommand("/usr/sbin/zfs")

var Zpool = newAllowedCommand("/usr/sbin/zpool")

var Zypper = newAllowedCommand("/usr/bin/zypper")

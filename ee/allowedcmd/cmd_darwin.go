//go:build darwin

package allowedcmd

var Airport = newAllowedCommand("/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport")

var Bioutil = newAllowedCommand("/usr/bin/bioutil")

var Bputil = newAllowedCommand("/usr/bin/bputil")

var Brew = newAllowedCommand("/opt/homebrew/bin/brew", "/usr/local/bin/brew").WithEnv("HOMEBREW_NO_AUTO_UPDATE=1")

var Diskutil = newAllowedCommand("/usr/sbin/diskutil")

var Echo = newAllowedCommand("/bin/echo")

var Falconctl = newAllowedCommand("/Applications/Falcon.app/Contents/Resources/falconctl")

var Fdesetup = newAllowedCommand("/usr/bin/fdesetup")

var Firmwarepasswd = newAllowedCommand("/usr/sbin/firmwarepasswd")

var Ifconfig = newAllowedCommand("/sbin/ifconfig")

var Ioreg = newAllowedCommand("/usr/sbin/ioreg")

var Launchctl = newAllowedCommand("/bin/launchctl")

var Lsof = newAllowedCommand("/usr/sbin/lsof")

var Mdfind = newAllowedCommand("/usr/bin/mdfind")

var Mdmclient = newAllowedCommand("/usr/libexec/mdmclient")

var MicrosoftDefenderATP = newAllowedCommand("/usr/local/bin/mdatp")

var Netstat = newAllowedCommand("/usr/sbin/netstat")

var NixEnv = newAllowedCommand("/nix/var/nix/profiles/default/bin/nix-env")

var Open = newAllowedCommand("/usr/bin/open")

var Pkgutil = newAllowedCommand("/usr/sbin/pkgutil")

var Powermetrics = newAllowedCommand("/usr/bin/powermetrics")

var Profiles = newAllowedCommand("/usr/bin/profiles")

var Ps = newAllowedCommand("/bin/ps")

var Pwpolicy = newAllowedCommand("/usr/bin/pwpolicy")

var Remotectl = newAllowedCommand("/usr/libexec/remotectl")

var Repcli = newAllowedCommand("/Applications/VMware Carbon Black Cloud/repcli.bundle/Contents/MacOS/repcli")

var Scutil = newAllowedCommand("/usr/sbin/scutil")

var Security = newAllowedCommand("/usr/bin/security")

var Socketfilterfw = newAllowedCommand("/usr/libexec/ApplicationFirewall/socketfilterfw")

var Softwareupdate = newAllowedCommand("/usr/sbin/softwareupdate")

var SystemProfiler = newAllowedCommand("/usr/sbin/system_profiler")

var Tmutil = newAllowedCommand("/usr/bin/tmutil")

var ZerotierCli = newAllowedCommand("/usr/local/bin/zerotier-cli")

var Zfs = newAllowedCommand("/usr/sbin/zfs")

var Zpool = newAllowedCommand("/usr/sbin/zpool")

var Zscli = newAllowedCommand("/Applications/Zscaler/Zscaler.app/Contents/PlugIns/zscli")

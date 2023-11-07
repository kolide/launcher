//go:build linux
// +build linux

package allowedpaths

var knownPaths = map[string]map[string]bool{
	"apt": {
		"/usr/bin/apt": true,
	},
	"cryptsetup": {
		"/usr/sbin/cryptsetup": true,
		"/sbin/cryptsetup":     true,
	},
	"dnf": {
		"/usr/bin/dnf": true,
	},
	"dpkg": {
		"/usr/bin/dpkg": true,
	},
	"lsof": {
		"/usr/bin/lsof": true,
	},
	"falcon-kernel-check": {
		"/opt/CrowdStrike/falcon-kernel-check": true,
	},
	"falconctl": {
		"/opt/CrowdStrike/falconctl": true,
	},
	"gnome-extensions": {
		"/usr/bin/gnome-extensions": true,
	},
	"gsettings": {
		"/usr/bin/gsettings": true,
	},
	"ifconfig": {
		"/usr/sbin/ifconfig": true,
	},
	"ip": {
		"/usr/sbin/ip": true,
	},
	"loginctl": {
		"/usr/bin/loginctl": true,
	},
	"lsblk": {
		"/bin/lsblk",
		"/usr/bin/lsblk",
	},
	"mdmclient": {
		"/usr/libexec/mdmclient": true,
	},
	"nmcli": {
		"/usr/bin/nmcli": true,
	},
	"notify-send": {
		"/usr/bin/notify-send": true,
	},
	"pacman": {
		"/usr/bin/pacman": true,
	},
	"ps": {
		"/usr/bin/ps": true,
	},
	"repcli": {
		"/opt/carbonblack/psc/bin/repcli": true,
	},
	"rpm": {
		"/usr/bin/rpm": true,
		"/bin/rpm":     true,
	},
	"systemctl": {
		"/usr/bin/systemctl": true,
	},
	"x-www-browser": {
		"/usr/bin/x-www-browser": true,
	},
	"xdg-open": {
		"/usr/bin/xdg-open": true,
	},
	"xrdb": {
		"/usr/bin/xrdb": true,
	},
	"zerotier-cli": {
		"/usr/local/bin/zerotier-cli": true,
	},
	"zfs": {
		"/usr/sbin/zfs": true,
	},
	"zpool": {
		"/usr/sbin/zpool": true,
	},
}

var knownPathPrefixes = []string{
	"/bin",
	"/nix/store", // NixOS support
	"/usr/bin",
	"/usr/local/bin",
	"/usr/sbin",
}

//go:build linux
// +build linux

package allowedpaths

var knownPaths = map[string]map[string]bool{
	"cryptsetup": {
		"/usr/sbin/cryptsetup": true,
		"/sbin/cryptsetup":     true,
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
	"mdmclient": {
		"/usr/libexec/mdmclient": true,
	},
	"notify-send": {
		"/usr/bin/notify-send": true,
	},
	"ps": {
		"/usr/bin/ps": true,
	},
	"rpm": {
		"/bin/rpm": true,
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

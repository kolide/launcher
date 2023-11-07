//go:build darwin
// +build darwin

package allowedpaths

var knownPaths = map[string]map[string]bool{
	"bioutil": {
		"/usr/bin/bioutil": true,
	},
	"diskutil": {
		"/usr/sbin/diskutil": true,
	},
	"falconctl": {
		"/Applications/Falcon.app/Contents/Resources/falconctl": true,
	},
	"firmwarepasswd": {
		"/usr/sbin/firmwarepasswd": true,
	},
	"ifconfig": {
		"/sbin/ifconfig": true,
	},
	"launchctl": {
		"/bin/launchctl": true,
	},
	"lsof": {
		"/usr/sbin/lsof": true,
	},
	"mdfind": {
		"/usr/bin/mdfind": true,
	},
	"netstat": {
		"/usr/sbin/netstat": true,
	},
	"open": {
		"/usr/bin/open": true,
	},
	"pkgutil": {
		"/usr/sbin/pkgutil": true,
	},
	"powermetrics": {
		"/usr/bin/powermetrics": true,
	},
	"profiles": {
		"/usr/bin/profiles": true,
	},
	"ps": {
		"/bin/ps": true,
	},
	"pwpolicy": {
		"/usr/bin/pwpolicy": true,
	},
	"remotectl": {
		"/usr/libexec/remotectl": true,
	},
	"repcli": {
		"/Applications/VMware Carbon Black Cloud/repcli.bundle/Contents/MacOS/repcli": true,
	},
	"scutil": {
		"/usr/sbin/scutil": true,
	},
	"softwareupdate": {
		"/usr/sbin/softwareupdate": true,
	},
	"system_profiler": {
		"/usr/sbin/system_profiler": true,
	},
	"tmutil": {
		"/usr/bin/tmutil": true,
	},
	"zerotier-cli": {
		"/usr/local/bin/zerotier-cli": true,
	},
}

var knownPathPrefixes = []string{
	"/bin",
	"/usr/bin",
	"/usr/libexec",
	"/usr/local/bin",
	"/usr/sbin",
}
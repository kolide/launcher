//go:build linux
// +build linux

package checkups

import "github.com/kolide/launcher/pkg/allowedpaths"

func listCommands() []networkCommand {
	return []networkCommand{
		{
			cmd:  allowedpaths.Ifconfig,
			args: []string{"-a"},
		},
		{
			cmd:  allowedpaths.Ip,
			args: []string{"-N", "-d", "-h", "-a", "address"},
		},
		{
			cmd:  allowedpaths.Ip,
			args: []string{"-N", "-d", "-h", "-a", "route"},
		},
	}
}

func listFiles() []string {
	return []string{
		"/etc/nsswitch.conf",
		"/etc/hosts",
		"/etc/resolv.conf",
	}
}

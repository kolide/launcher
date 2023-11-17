//go:build linux
// +build linux

package checkups

import "github.com/kolide/launcher/ee/allowedcmd"

func listCommands() []networkCommand {
	return []networkCommand{
		{
			cmd:  allowedcmd.Ifconfig,
			args: []string{"-a"},
		},
		{
			cmd:  allowedcmd.Ip,
			args: []string{"-N", "-d", "-h", "-a", "address"},
		},
		{
			cmd:  allowedcmd.Ip,
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

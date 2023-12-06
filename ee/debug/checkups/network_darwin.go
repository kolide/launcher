//go:build darwin
// +build darwin

package checkups

import "github.com/kolide/launcher/ee/allowedcmd"

func listCommands() []networkCommand {
	return []networkCommand{
		{
			cmd:  allowedcmd.Ifconfig,
			args: []string{"-a"},
		},
		{
			cmd:  allowedcmd.Netstat,
			args: []string{"-nr"},
		},
	}
}

func listFiles() []string {
	return []string{
		"/etc/hosts",
		"/etc/resolv.conf",
	}
}

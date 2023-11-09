//go:build darwin
// +build darwin

package checkups

import "github.com/kolide/launcher/pkg/allowedpaths"

func listCommands() []networkCommand {
	return []networkCommand{
		{
			cmd:  allowedpaths.Ifconfig,
			args: []string{"-a"},
		},
		{
			cmd:  allowedpaths.Netstat,
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

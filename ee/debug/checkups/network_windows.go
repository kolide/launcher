//go:build windows
// +build windows

package checkups

import "github.com/kolide/launcher/ee/allowedcmd"

func listCommands() []networkCommand {
	return []networkCommand{
		{
			cmd:  allowedcmd.Ipconfig,
			args: []string{"/all"},
		},
	}
}

func listFiles() []string {
	return []string{}
}

//go:build windows
// +build windows

package checkups

import "github.com/kolide/launcher/pkg/allowedpaths"

func listCommands() []networkCommand {
	return []networkCommand{
		{
			cmd:  allowedpaths.Ipconfig,
			args: []string{"/all"},
		},
	}
}

func listFiles() []string {
	return []string{}
}

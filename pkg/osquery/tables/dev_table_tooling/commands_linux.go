//go:build linux
// +build linux

package dev_table_tooling

func GetAllowedCommands() map[string]AllowedCommand {
	cmds := map[string]AllowedCommand{
		"ifconfig": {
			binPaths: []string{"ifconfig"},
			args:     []string{},
		},
	}
	return cmds
}

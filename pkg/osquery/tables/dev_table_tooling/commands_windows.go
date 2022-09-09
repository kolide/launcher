//go:build windows
// +build windows

package dev_table_tooling

func GetAllowedCommands() map[string]AllowedCommand {
	cmds := map[string]AllowedCommand{
		"hostname": {
			binPaths: []string{"hostname"},
			args:     []string{},
		},
	}
	return cmds
}

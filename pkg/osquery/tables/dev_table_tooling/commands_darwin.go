//go:build darwin
// +build darwin

package dev_table_tooling

func GetAllowedCommands() map[string]AllowedCommand {
	cmds := map[string]AllowedCommand{
		"system_profiler": {
			binPaths: []string{"/usr/sbin/system_profiler"},
			args:     []string{"SPSoftwareDataType", "SPNetworkDataType"},
		},
		"diskutil": {
			binPaths: []string{"diskutil"},
			args:     []string{"info", "-all"},
		},
	}
	return cmds
}

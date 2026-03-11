//go:build windows

package dev_table_tooling

import "github.com/kolide/launcher/v2/ee/allowedcmd"

var allowedCommands = map[string]allowedCommand{
	"echo": {
		bin:  allowedcmd.Echo,
		args: []string{"hello"},
	},
	"cb_repcli": {
		bin:  allowedcmd.Repcli,
		args: []string{"status"},
	},
}

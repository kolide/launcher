//go:build windows
// +build windows

package dev_table_tooling

import "github.com/kolide/launcher/pkg/allowedcmd"

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

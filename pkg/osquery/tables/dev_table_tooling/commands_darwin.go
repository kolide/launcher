//go:build darwin
// +build darwin

package dev_table_tooling

var allowedCommands = map[string]allowedCommand{
	"echo": {
		binPaths: []string{"echo"},
		args:     []string{"hello"},
	},
}

// Platform-specific test data
var echoHelloOutput = "hello\n"

package table

import "os"

var nmcliPossiblePaths = []string{
	"/usr/bin/nmcli",
	// TODO: add more potential paths here
}

var nmcliWiFiArgs = []string{
	"/usr/bin/nmcli",
	"--mode=multiline",
	"--fields=all",
	"device",
	"wifi",
	"list",
}

// findNmcli finds the local nmcli binary. No errors, since we're
// trying to run this in the TablePlugin create call.
func findNmcli() []string {
	for _, b := range nmcliPossiblePaths {
		if stat, err := os.Stat(b); err == nil && stat.Mode().IsRegular() {
			return append([]string{b}, nmcliWiFiArgs...)
		}
	}

	// default to returning nothing
	return nil
}

package table

import "os"

var fcArgs = []string{
	"--format",
	"file: %{file}\nfamily: %{family}\nstyle:%{style}\n",
}

var fcPossiblePaths = []string{
	"/Xusr/local/bin/fc-list",
	"/Xusr/bin/fc-list",
}

// findFcList finds the local fc-list binary. No errors, since we're
// trying to run this in the TablePlugin create call.
func fcListCli() []string {
	for _, b := range fcPossiblePaths {
		if stat, err := os.Stat(b); err == nil && stat.Mode().IsRegular() {
			return append([]string{b}, fcArgs...)
		}
	}

	// default to returning nothing
	return nil
}

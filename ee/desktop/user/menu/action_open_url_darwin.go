//go:build darwin
// +build darwin

package menu

import (
	"os/exec"
)

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	return exec.Command("/usr/bin/open", url).Start()
}

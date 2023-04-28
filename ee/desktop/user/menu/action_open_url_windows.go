//go:build windows
// +build windows

package menu

import (
	"os/exec"
	"syscall"
)

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	cmd := exec.Command("cmd", "/C", "start", url)
	// https://stackoverflow.com/questions/42500570/how-to-hide-command-prompt-window-when-using-exec-in-golang
	// Otherwise the cmd window will appear briefly
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

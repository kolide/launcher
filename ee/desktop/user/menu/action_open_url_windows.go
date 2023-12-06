//go:build windows
// +build windows

package menu

import (
	"context"
	"fmt"
	"syscall"

	"github.com/kolide/launcher/ee/allowedcmd"
)

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	cmd, err := allowedcmd.CommandPrompt(context.TODO(), "/C", "start", url)
	if err != nil {
		return fmt.Errorf("creating command: %w", err)
	}

	// https://stackoverflow.com/questions/42500570/how-to-hide-command-prompt-window-when-using-exec-in-golang
	// Otherwise the cmd window will appear briefly
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

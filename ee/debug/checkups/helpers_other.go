//go:build !windows

package checkups

import (
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {
}

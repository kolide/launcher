//go:build !darwin
// +build !darwin

package tablehelpers

import (
	"os/exec"
)

// Disclaimed is a no-op for everything but darwin
func Disclaimed(disclaimCmdName string) ExecOps {
	return func(cmd *exec.Cmd) error {
		return nil
	}
}

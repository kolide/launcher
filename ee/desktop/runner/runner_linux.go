//go:build linux
// +build linux

package runner

import (
	"fmt"
	"os/exec"
)

func runAsUser(uid string, cmd *exec.Cmd) error {
	return fmt.Errorf("not implemented")
}

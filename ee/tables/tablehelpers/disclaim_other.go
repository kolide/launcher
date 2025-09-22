//go:build !darwin
// +build !darwin

package tablehelpers

import (
	"context"
	"os/exec"
)

// Disclaimed is a no-op for everything but darwin
func Disclaimed(_ context.Context, _ string) ExecOps {
	return func(cmd *exec.Cmd) error {
		return nil
	}
}

//go:build linux

package nativemessaging

import (
	"context"
	"errors"

	"github.com/shirou/gopsutil/v4/process"
)

func validateBrowser(ctx context.Context, proc *process.Process) error {
	return errors.New("not implemented")
}

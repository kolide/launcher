//go:build darwin

package nativemessaging

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/process"
)

var allowlistedChromePaths = map[string]struct{}{
	`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`: {},
}

func validateBrowser(ctx context.Context, proc *process.Process) error {
	browserProcessExe, err := proc.ExeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting executable for browser process: %w", err)
	}

	// The calling process must have a path in our allowlist
	if _, found := allowlistedChromePaths[browserProcessExe]; !found {
		return fmt.Errorf("path %s for browser process not in allowlisted chrome paths", browserProcessExe)
	}

	return nil
}

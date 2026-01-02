//go:build darwin || linux

package listener

import (
	"fmt"
	"os"
)

func setSocketPermissions(socketPath string) error {
	if err := os.Chmod(socketPath, 0600); err != nil {
		return fmt.Errorf("chmodding %s: %w", socketPath, err)
	}
	return nil
}

//go:build darwin || linux

package listener

import (
	"fmt"
	"os"
)

func setPipePermissions(pipePath string) error {
	if err := os.Chmod(pipePath, 0600); err != nil {
		return fmt.Errorf("chmodding %s: %w", pipePath, err)
	}
	return nil
}

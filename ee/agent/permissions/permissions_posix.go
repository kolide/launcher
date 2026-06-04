//go:build !windows

package permissions

import (
	"fmt"
	"os"
)

func RestrictFileAccessToRootOnly(filePathToRestrict string) error {
	if err := os.Chmod(filePathToRestrict, 0600); err != nil {
		return fmt.Errorf("chmodding %s: %w", filePathToRestrict, err)
	}
	return nil
}

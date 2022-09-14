package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func DesktopSocketPath(pid int) string {
	const socketBaseName = "launcher_desktop.sock"

	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\%d_%s`, pid, socketBaseName)
	}

	return filepath.Join(os.TempDir(), fmt.Sprintf("%d_%s", pid, socketBaseName))
}

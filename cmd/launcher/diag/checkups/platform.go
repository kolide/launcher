package checkups

import (
	"fmt"
	"io"
	"runtime"

	"golang.org/x/exp/slices"
)

type Platform struct {
}

func (c *Platform) Name() string {
	return "Platform"
}

func (c *Platform) Run(short io.Writer) (string, error) {
	return checkupPlatform(runtime.GOOS)
}

// checkupPlatform verifies that the current OS is supported by launcher
func checkupPlatform(os string) (string, error) {
	if slices.Contains([]string{"windows", "darwin", "linux"}, os) {
		return fmt.Sprintf("Platform: %s", os), nil
	}
	return "", fmt.Errorf("Unsupported platform:\t%s", os)
}

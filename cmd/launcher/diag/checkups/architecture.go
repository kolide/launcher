package checkups

import (
	"fmt"
	"io"
	"runtime"

	"golang.org/x/exp/slices"
)

type Arch struct {
}

func (c *Arch) Name() string {
	return "Architecture"
}

func (c *Arch) Run(short io.Writer) (string, error) {
	return checkupArch(runtime.GOARCH)
}

// checkupArch verifies that the current architecture is supported by launcher
func checkupArch(arch string) (string, error) {
	if slices.Contains([]string{"386", "amd64", "arm64"}, arch) {
		return fmt.Sprintf("Architecture: %s", arch), nil
	}
	return "", fmt.Errorf("Unsupported architecture:\t%s", arch)
}

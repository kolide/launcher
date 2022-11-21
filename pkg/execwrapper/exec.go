//go:build !windows
// +build !windows

package execwrapper

import (
	"context"
	"os"
	"syscall"
)

func Exec(ctx context.Context, argv0 string, argv []string, envv []string, in *os.File) (err error) {
	return syscall.Exec(argv0, argv, envv)
}
